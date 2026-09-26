package router_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/handler"
	"github.com/gbtreehole/backend/internal/middleware"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
	"github.com/gbtreehole/backend/internal/router"
	"github.com/gbtreehole/backend/internal/service"
)

type apiResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"message"`
	Data json.RawMessage `json:"data"`
}

func setupServer(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	identityRepo := repository.NewIdentityRepository(db)
	postRepo := repository.NewPostRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	tagRepo := repository.NewTagRepository(db)
	likeRepo := repository.NewLikeRepository(db)
	favRepo := repository.NewFavoriteRepository(db)
	sensitiveRepo := repository.NewSensitiveWordRepository(db)
	reviewRepo := repository.NewReviewQueueRepository(db)

	tokenService := service.NewTokenService("test-secret", 60)
	identityService := service.NewIdentityService(identityRepo, tokenService, logger)
	tagService := service.NewTagService(tagRepo)
	sensitiveService := service.NewSensitiveWordService(sensitiveRepo)
	reviewService := service.NewReviewService(reviewRepo, postRepo, commentRepo, logger)
	postService := service.NewPostService(postRepo, tagService, sensitiveService, reviewService, logger)
	commentService := service.NewCommentService(commentRepo, postRepo, sensitiveService, reviewService, logger)
	likeService := service.NewLikeService(likeRepo, postRepo, commentRepo, logger)
	favoriteService := service.NewFavoriteService(favRepo, postRepo, logger)

	authHandler := handler.NewAuthHandler(identityService, logger)
	postHandler := handler.NewPostHandler(postService, likeService, favoriteService, logger)
	commentHandler := handler.NewCommentHandler(commentService, likeService, logger)
	tagHandler := handler.NewTagHandler(tagService, logger)
	likeHandler := handler.NewLikeHandler(likeService, logger)
	favoriteHandler := handler.NewFavoriteHandler(favoriteService, likeService, logger)
	adminHandler := handler.NewAdminHandler(reviewService, postService, sensitiveService, tagService, logger)
	identityMW := middleware.NewIdentityAuthMiddleware(tokenService)
	sensitiveMW := middleware.NewSensitiveWordMiddleware(sensitiveService, logger)

	return router.New(logger, authHandler, postHandler, commentHandler, tagHandler, likeHandler, favoriteHandler, adminHandler, identityMW, sensitiveMW), db
}

func do(t *testing.T, r *gin.Engine, method, path, token string, body any) (apiResp, int) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	raw, _ := io.ReadAll(w.Body)
	var resp apiResp
	_ = json.Unmarshal(raw, &resp)
	return resp, w.Code
}

func loginIdentity(t *testing.T, r *gin.Engine) string {
	t.Helper()
	resp, code := do(t, r, http.MethodPost, "/api/v1/auth/identities", "", map[string]string{"nickname": ""})
	if code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("create identity failed: code=%d resp=%s", code, resp.Msg)
	}
	var data struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode identity: %v", err)
	}
	return data.Token
}

func createPost(t *testing.T, r *gin.Engine, token, content string) uint {
	t.Helper()
	resp, code := do(t, r, http.MethodPost, "/api/v1/posts", token, map[string]any{"content": content})
	if code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("create post failed: code=%d resp=%s", code, resp.Msg)
	}
	var data struct {
		Post struct {
			ID uint `json:"id"`
		} `json:"post"`
	}
	_ = json.Unmarshal(resp.Data, &data)
	return data.Post.ID
}

func TestFavoriteEndToEnd(t *testing.T) {
	r, db := setupServer(t)
	tokenA := loginIdentity(t, r)
	tokenB := loginIdentity(t, r)
	postID := createPost(t, r, tokenA, "一条值得回看的树洞")

	// 未登录不能收藏
	if _, code := do(t, r, http.MethodPost, "/api/v1/posts/"+itoa(postID)+"/favorite", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", code)
	}

	// A 收藏
	resp, code := do(t, r, http.MethodPost, "/api/v1/posts/"+itoa(postID)+"/favorite", tokenA, nil)
	if code != 200 || resp.Code != 0 {
		t.Fatalf("favorite failed: %d %s", code, resp.Msg)
	}
	var fav struct {
		Favorited     bool  `json:"favorited"`
		FavoriteCount int64 `json:"favoriteCount"`
	}
	_ = json.Unmarshal(resp.Data, &fav)
	if !fav.Favorited || fav.FavoriteCount != 1 {
		t.Fatalf("unexpected favorite result: %+v", fav)
	}

	// 重复收藏：幂等，仍返回 true/1，库里只有一条记录
	resp, _ = do(t, r, http.MethodPost, "/api/v1/posts/"+itoa(postID)+"/favorite", tokenA, nil)
	_ = json.Unmarshal(resp.Data, &fav)
	if !fav.Favorited || fav.FavoriteCount != 1 {
		t.Fatalf("duplicate favorite should be idempotent: %+v", fav)
	}
	var rows int64
	db.Model(&model.Favorite{}).Where("post_id = ?", postID).Count(&rows)
	if rows != 1 {
		t.Fatalf("expected 1 favorite row, got %d", rows)
	}

	// 帖子列表上的 favorited 标记与 favoriteCount 与服务端一致（A 视角）
	resp, _ = do(t, r, http.MethodGet, "/api/v1/posts", tokenA, nil)
	var page struct {
		Items []struct {
			ID            uint `json:"id"`
			FavoriteCount int  `json:"favoriteCount"`
			Favorited     bool `json:"favorited"`
		} `json:"items"`
	}
	_ = json.Unmarshal(resp.Data, &page)
	if len(page.Items) != 1 || !page.Items[0].Favorited || page.Items[0].FavoriteCount != 1 {
		t.Fatalf("post list favorite mark mismatch: %+v", page.Items)
	}

	// B 看不到 A 的收藏：标记为 false，但公开计数为 1
	resp, _ = do(t, r, http.MethodGet, "/api/v1/posts", tokenB, nil)
	_ = json.Unmarshal(resp.Data, &page)
	if len(page.Items) != 1 || page.Items[0].Favorited || page.Items[0].FavoriteCount != 1 {
		t.Fatalf("identity isolation violated: %+v", page.Items)
	}
	resp, _ = do(t, r, http.MethodGet, "/api/v1/favorites", tokenB, nil)
	_ = json.Unmarshal(resp.Data, &page)
	if len(page.Items) != 0 {
		t.Fatalf("B should not see A's favorites: %+v", page.Items)
	}

	// A 的收藏列表有 1 条
	resp, _ = do(t, r, http.MethodGet, "/api/v1/favorites", tokenA, nil)
	_ = json.Unmarshal(resp.Data, &page)
	if len(page.Items) != 1 || page.Items[0].ID != postID {
		t.Fatalf("A favorites mismatch: %+v", page.Items)
	}

	// 按最近收藏时间排序：A 再收藏第二个帖子，它应排在最前
	time.Sleep(10 * time.Millisecond)
	secondID := createPost(t, r, tokenB, "第二条")
	_, _ = do(t, r, http.MethodPost, "/api/v1/posts/"+itoa(secondID)+"/favorite", tokenA, nil)
	resp, _ = do(t, r, http.MethodGet, "/api/v1/favorites", tokenA, nil)
	_ = json.Unmarshal(resp.Data, &page)
	if len(page.Items) != 2 || page.Items[0].ID != secondID {
		t.Fatalf("favorites should be ordered by latest favorited time: %+v", page.Items)
	}

	// 审核屏蔽第一个帖子：A 的收藏列表隐藏它，但收藏关系保留
	if err := db.Model(&model.Post{}).Where("id = ?", postID).Update("status", constants.PostStatusRejected).Error; err != nil {
		t.Fatalf("reject post: %v", err)
	}
	resp, _ = do(t, r, http.MethodGet, "/api/v1/favorites", tokenA, nil)
	_ = json.Unmarshal(resp.Data, &page)
	var total struct {
		Total int64 `json:"total"`
	}
	_ = json.Unmarshal(resp.Data, &total)
	if total.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != secondID {
		t.Fatalf("blocked post should be hidden from favorites: total=%d items=%+v", total.Total, page.Items)
	}
	db.Model(&model.Favorite{}).Where("identity_id = ? AND post_id = ?", 1, postID).Count(&rows)
	if rows != 1 {
		t.Fatalf("favorite relation must be kept while blocked, rows=%d", rows)
	}

	// 审核放行：重新出现
	if err := db.Model(&model.Post{}).Where("id = ?", postID).Update("status", constants.PostStatusPublished).Error; err != nil {
		t.Fatalf("approve post: %v", err)
	}
	resp, _ = do(t, r, http.MethodGet, "/api/v1/favorites", tokenA, nil)
	_ = json.Unmarshal(resp.Data, &total)
	var raw struct {
		Items []struct{ ID uint } `json:"items"`
	}
	_ = json.Unmarshal(resp.Data, &raw)
	if total.Total != 2 || len(raw.Items) != 2 {
		t.Fatalf("favorites should reappear after approve: total=%d len=%d", total.Total, len(raw.Items))
	}

	// A 取消收藏：计数归零；再次取消仍幂等返回 0
	resp, _ = do(t, r, http.MethodDelete, "/api/v1/posts/"+itoa(postID)+"/favorite", tokenA, nil)
	_ = json.Unmarshal(resp.Data, &fav)
	if fav.Favorited || fav.FavoriteCount != 0 {
		t.Fatalf("unfavorite result mismatch: %+v", fav)
	}
	resp, _ = do(t, r, http.MethodDelete, "/api/v1/posts/"+itoa(postID)+"/favorite", tokenA, nil)
	_ = json.Unmarshal(resp.Data, &fav)
	if fav.Favorited || fav.FavoriteCount != 0 {
		t.Fatalf("duplicate unfavorite should be idempotent: %+v", fav)
	}
	db.Model(&model.Favorite{}).Where("post_id = ?", postID).Count(&rows)
	if rows != 0 {
		t.Fatalf("expected 0 rows after unfavorite, got %d", rows)
	}
}

func itoa(n uint) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
