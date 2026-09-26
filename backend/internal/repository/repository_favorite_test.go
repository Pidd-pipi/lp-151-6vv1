package repository

import (
	"testing"
	"time"

	"github.com/gbtreehole/backend/internal/model"
)

func TestFavoriteRepository(t *testing.T) {
	db := newTestDB(t)
	identityRepo := NewIdentityRepository(db)
	postRepo := NewPostRepository(db)
	favRepo := NewFavoriteRepository(db)

	identity := &model.UserIdentity{IdentityKey: "key-fav", Nickname: "收藏者", Avatar: "a.png", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := identityRepo.Create(identity); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	makePost := func(content string, status int) *model.Post {
		p := &model.Post{IdentityID: identity.ID, Content: content, Status: status, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := postRepo.Create(p); err != nil {
			t.Fatalf("create post: %v", err)
		}
		return p
	}
	p1 := makePost("p1", 1)
	p2 := makePost("p2", 1)
	p3 := makePost("p3", 3) // 被审核屏蔽

	// 初始计数为 0
	if count, err := favRepo.CountByPost(p1.ID); err != nil || count != 0 {
		t.Fatalf("initial count = %d, err = %v, want 0", count, err)
	}
	if err := favRepo.Create(&model.Favorite{IdentityID: identity.ID, PostID: p1.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("create favorite: %v", err)
	}
	// 唯一约束：重复插入必须失败而不是产生重复记录
	if err := favRepo.Create(&model.Favorite{IdentityID: identity.ID, PostID: p1.ID, CreatedAt: time.Now()}); err == nil {
		t.Fatal("expected duplicate favorite to fail")
	} else if !IsDuplicateErr(err) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	var rows int64
	if err := db.Model(&model.Favorite{}).Where("identity_id = ? AND post_id = ?", identity.ID, p1.ID).Count(&rows).Error; err != nil || rows != 1 {
		t.Fatalf("expected exactly 1 favorite row, got %d (err=%v)", rows, err)
	}

	// 计数
	if count, err := favRepo.CountByPost(p1.ID); err != nil || count != 1 {
		t.Fatalf("count = %d, err = %v, want 1", count, err)
	}

	// 不同身份可各自收藏同一帖子
	other := &model.UserIdentity{IdentityKey: "key-other", Nickname: "其他人", Avatar: "b.png", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := identityRepo.Create(other); err != nil {
		t.Fatalf("create other identity: %v", err)
	}
	if err := favRepo.Create(&model.Favorite{IdentityID: other.ID, PostID: p1.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("other create favorite: %v", err)
	}
	if count, _ := favRepo.CountByPost(p1.ID); count != 2 {
		t.Fatalf("count after other favorite = %d, want 2", count)
	}

	// 收藏状态批量查询按身份隔离
	marked, err := favRepo.IsFavorited(identity.ID, []uint{p1.ID, p2.ID})
	if err != nil {
		t.Fatalf("is favorited: %v", err)
	}
	if !marked[p1.ID] || marked[p2.ID] {
		t.Fatalf("unexpected favorite marks: %v", marked)
	}

	// 收藏时间倒序：p2 收藏时间晚于 p1，屏蔽帖 p3 也收藏但应被列表过滤
	time.Sleep(10 * time.Millisecond)
	if err := favRepo.Create(&model.Favorite{IdentityID: identity.ID, PostID: p2.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("favorite p2: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := favRepo.Create(&model.Favorite{IdentityID: identity.ID, PostID: p3.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("favorite p3: %v", err)
	}
	items, total, err := favRepo.ListFavorites(identity.ID, 1, 1, 10)
	if err != nil {
		t.Fatalf("list favorites: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected 2 visible favorites, got total=%d len=%d", total, len(items))
	}
	if items[0].PostID != p2.ID || items[1].PostID != p1.ID {
		t.Fatalf("favorites not ordered by created_at desc: %d then %d", items[0].PostID, items[1].PostID)
	}
	// 收藏关系仍然保留：不经过帖子状态过滤时记录还在
	if count, _ := favRepo.CountByPost(p3.ID); count != 1 {
		t.Fatalf("blocked post favorite relation should be kept, count=%d", count)
	}

	// 取消收藏
	if rows, err := favRepo.Delete(identity.ID, p1.ID); err != nil || rows != 1 {
		t.Fatalf("delete favorite: rows=%d err=%v, want 1", rows, err)
	}
	if marked, _ := favRepo.IsFavorited(identity.ID, []uint{p1.ID}); marked[p1.ID] {
		t.Fatal("favorite should be removed")
	}
	// 重复删除返回 0 行
	if rows, err := favRepo.Delete(identity.ID, p1.ID); err != nil || rows != 0 {
		t.Fatalf("duplicate delete: rows=%d err=%v, want 0", rows, err)
	}
	// 其他身份的收藏不受影响
	if marked, _ := favRepo.IsFavorited(other.ID, []uint{p1.ID}); !marked[p1.ID] {
		t.Fatal("other identity favorite must remain")
	}
}
