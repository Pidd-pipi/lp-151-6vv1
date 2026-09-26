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

func newFavoriteServiceDB(t *testing.T) (*gorm.DB, repository.FavoriteRepository, repository.PostRepository, repository.IdentityRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.UserIdentity{}, &model.Post{}, &model.Tag{}, &model.PostTag{}, &model.Favorite{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db, repository.NewFavoriteRepository(db), repository.NewPostRepository(db), repository.NewIdentityRepository(db)
}

func TestFavoriteServiceToggle(t *testing.T) {
	db, favRepo, postRepo, identityRepo := newFavoriteServiceDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewFavoriteService(favRepo, postRepo, logger)

	identity := &model.UserIdentity{IdentityKey: "k1", Nickname: "甲", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := identityRepo.Create(identity); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	post := &model.Post{IdentityID: identity.ID, Content: "帖子", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := postRepo.Create(post); err != nil {
		t.Fatalf("create post: %v", err)
	}

	// 收藏
	favorited, count, err := svc.Add(identity.ID, post.ID)
	if err != nil || !favorited || count != 1 {
		t.Fatalf("add favorite: favorited=%v count=%d err=%v", favorited, count, err)
	}
	stored, _ := postRepo.FindByID(post.ID)
	if stored.FavoriteCount != 1 {
		t.Fatalf("post favorite count = %d, want 1", stored.FavoriteCount)
	}

	// 重复收藏幂等：不新增记录、计数不增加
	favorited, count, err = svc.Add(identity.ID, post.ID)
	if err != nil || !favorited || count != 1 {
		t.Fatalf("duplicate add: favorited=%v count=%d err=%v", favorited, count, err)
	}
	var rows int64
	db.Model(&model.Favorite{}).Where("identity_id = ? AND post_id = ?", identity.ID, post.ID).Count(&rows)
	if rows != 1 {
		t.Fatalf("expected 1 favorite row, got %d", rows)
	}
	stored, _ = postRepo.FindByID(post.ID)
	if stored.FavoriteCount != 1 {
		t.Fatalf("post favorite count after duplicate add = %d, want 1", stored.FavoriteCount)
	}

	// 快速重复取消两次：计数不能变成负数
	for i := 0; i < 2; i++ {
		favorited, count, err = svc.Remove(identity.ID, post.ID)
		if err != nil || favorited || count != 0 {
			t.Fatalf("remove #%d: favorited=%v count=%d err=%v", i+1, favorited, count, err)
		}
	}
	stored, _ = postRepo.FindByID(post.ID)
	if stored.FavoriteCount != 0 {
		t.Fatalf("post favorite count after remove = %d, want 0", stored.FavoriteCount)
	}

	// 收藏不存在的帖子
	if _, _, err := svc.Add(identity.ID, 9999); err != ErrPostNotFound {
		t.Fatalf("expected ErrPostNotFound, got %v", err)
	}
}

func TestFavoriteServiceListIsolationAndReview(t *testing.T) {
	_, favRepo, postRepo, identityRepo := newFavoriteServiceDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewFavoriteService(favRepo, postRepo, logger)

	ida := &model.UserIdentity{IdentityKey: "ka", Nickname: "甲", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	idb := &model.UserIdentity{IdentityKey: "kb", Nickname: "乙", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := identityRepo.Create(ida); err != nil {
		t.Fatalf("create ida: %v", err)
	}
	if err := identityRepo.Create(idb); err != nil {
		t.Fatalf("create idb: %v", err)
	}
	visible := &model.Post{IdentityID: ida.ID, Content: "可见帖", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	blocked := &model.Post{IdentityID: ida.ID, Content: "屏蔽帖", Status: constants.PostStatusRejected, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := postRepo.Create(visible); err != nil {
		t.Fatalf("create visible: %v", err)
	}
	if err := postRepo.Create(blocked); err != nil {
		t.Fatalf("create blocked: %v", err)
	}

	// 甲收藏两帖，乙只收藏可见帖
	if _, _, err := svc.Add(ida.ID, visible.ID); err != nil {
		t.Fatalf("a favorite visible: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, _, err := svc.Add(ida.ID, blocked.ID); err != nil {
		t.Fatalf("a favorite blocked: %v", err)
	}
	if _, _, err := svc.Add(idb.ID, visible.ID); err != nil {
		t.Fatalf("b favorite visible: %v", err)
	}

	// 甲的列表：屏蔽帖隐藏，可见帖出现，total 不含屏蔽帖
	posts, total, err := svc.List(ida.ID, 1, 20)
	if err != nil {
		t.Fatalf("list a: %v", err)
	}
	if total != 1 || len(posts) != 1 || posts[0].ID != visible.ID {
		t.Fatalf("a list = %+v total=%d, want only visible post", posts, total)
	}
	// 乙看不到甲的收藏
	bPosts, bTotal, err := svc.List(idb.ID, 1, 20)
	if err != nil {
		t.Fatalf("list b: %v", err)
	}
	if bTotal != 1 || len(bPosts) != 1 || bPosts[0].ID != visible.ID {
		t.Fatalf("b list = %+v total=%d, want only b's own favorite", bPosts, bTotal)
	}

	// 屏蔽期间收藏关系保留；审核放行后重新出现
	blocked.Status = constants.PostStatusPublished
	if err := postRepo.Update(blocked); err != nil {
		t.Fatalf("approve blocked: %v", err)
	}
	posts, total, err = svc.List(ida.ID, 1, 20)
	if err != nil {
		t.Fatalf("list a after approve: %v", err)
	}
	if total != 2 || len(posts) != 2 {
		t.Fatalf("expected 2 favorites after approve, got total=%d len=%d", total, len(posts))
	}
	// 最近收藏的屏蔽帖（后收藏）排在前面
	if posts[0].ID != blocked.ID || posts[1].ID != visible.ID {
		t.Fatalf("order mismatch: %d then %d", posts[0].ID, posts[1].ID)
	}
	// 乙的列表仍只有自己的 1 条
	_, bTotal, _ = svc.List(idb.ID, 1, 20)
	if bTotal != 1 {
		t.Fatalf("b total = %d, want 1", bTotal)
	}
}
