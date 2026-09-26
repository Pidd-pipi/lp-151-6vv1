package service

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newFavoriteServiceEnv(t *testing.T) (FavoriteService, repository.PostRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.UserIdentity{}, &model.Post{}, &model.Tag{}, &model.PostTag{}, &model.Favorite{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	postRepo := repository.NewPostRepository(db)
	favRepo := repository.NewFavoriteRepository(db)
	svc := NewFavoriteService(favRepo, postRepo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return svc, postRepo, db
}

func TestFavoriteServiceToggleAndCount(t *testing.T) {
	svc, postRepo, _ := newFavoriteServiceEnv(t)
	identity := &model.UserIdentity{ID: 1}
	post := &model.Post{IdentityID: identity.ID, Content: "hi", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := postRepo.Create(post); err != nil {
		t.Fatalf("create post: %v", err)
	}

	// 收藏
	favorited, count, err := svc.Toggle(identity.ID, post.ID)
	if err != nil || !favorited || count != 1 {
		t.Fatalf("favorite: favorited=%v count=%d err=%v", favorited, count, err)
	}

	// 模拟重复点击：再次切换会取消；再收藏回来，最终仍只有一条关系
	if favorited, _, err = svc.Toggle(identity.ID, post.ID); err != nil || favorited {
		t.Fatalf("second toggle should unfavorite, got favorited=%v err=%v", favorited, err)
	}
	if favorited, count, err = svc.Toggle(identity.ID, post.ID); err != nil || !favorited || count != 1 {
		t.Fatalf("third toggle should favorite once, got favorited=%v count=%d err=%v", favorited, count, err)
	}

	// 冗余计数与关系表一致
	got, err := postRepo.FindByID(post.ID)
	if err != nil {
		t.Fatalf("find post: %v", err)
	}
	if got.FavoriteCount != 1 {
		t.Fatalf("post favorite count mismatch: %d", got.FavoriteCount)
	}

	// 收藏不存在的帖子
	if _, _, err := svc.Toggle(identity.ID, 999); err != ErrPostNotFavoritable {
		t.Fatalf("expected ErrPostNotFavoritable, got %v", err)
	}
}

func TestFavoriteServiceListHiddenByReview(t *testing.T) {
	svc, postRepo, _ := newFavoriteServiceEnv(t)
	const identityID uint = 7

	visible := &model.Post{IdentityID: identityID, Content: "visible", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	blocked := &model.Post{IdentityID: identityID, Content: "blocked", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := postRepo.Create(visible); err != nil {
		t.Fatalf("create visible: %v", err)
	}
	if err := postRepo.Create(blocked); err != nil {
		t.Fatalf("create blocked: %v", err)
	}

	// 先收藏两篇，再审核屏蔽其中一篇
	if _, _, err := svc.Toggle(identityID, visible.ID); err != nil {
		t.Fatalf("favorite visible: %v", err)
	}
	if _, _, err := svc.Toggle(identityID, blocked.ID); err != nil {
		t.Fatalf("favorite blocked: %v", err)
	}
	blockedPost, err := postRepo.FindByID(blocked.ID)
	if err != nil {
		t.Fatalf("find blocked: %v", err)
	}
	blockedPost.Status = constants.PostStatusRejected
	if err := postRepo.Update(blockedPost); err != nil {
		t.Fatalf("reject post: %v", err)
	}

	items, total, err := svc.ListMine(identityID, 1, 20)
	if err != nil {
		t.Fatalf("list favorites: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].PostID != visible.ID {
		t.Fatalf("blocked post must be hidden while relation kept, got total=%d len=%d", total, len(items))
	}

	// 审核放行后自动重新出现
	blockedPost.Status = constants.PostStatusPublished
	if err := postRepo.Update(blockedPost); err != nil {
		t.Fatalf("approve post: %v", err)
	}
	items, total, _ = svc.ListMine(identityID, 1, 20)
	if total != 2 || len(items) != 2 {
		t.Fatalf("approved post should reappear, got total=%d len=%d", total, len(items))
	}
}

func TestFavoriteServiceIdentityIsolation(t *testing.T) {
	svc, postRepo, _ := newFavoriteServiceEnv(t)
	post := &model.Post{IdentityID: 1, Content: "shared", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := postRepo.Create(post); err != nil {
		t.Fatalf("create post: %v", err)
	}
	if _, _, err := svc.Toggle(1, post.ID); err != nil {
		t.Fatalf("identity 1 favorite: %v", err)
	}
	// 身份 2 未收藏，也看不到身份 1 的收藏
	items, total, err := svc.ListMine(2, 1, 20)
	if err != nil {
		t.Fatalf("list identity 2: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("identity 2 must not see others favorites, got total=%d len=%d", total, len(items))
	}
	got, err := svc.IsFavorited(2, []uint{post.ID})
	if err != nil || got[post.ID] {
		t.Fatalf("identity 2 should not have favorited status, map=%v err=%v", got, err)
	}
}
