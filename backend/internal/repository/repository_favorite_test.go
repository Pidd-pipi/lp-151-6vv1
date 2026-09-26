package repository

import (
	"testing"
	"time"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
)

func TestFavoriteRepository(t *testing.T) {
	db := newTestDB(t)
	favRepo := NewFavoriteRepository(db)
	postRepo := NewPostRepository(db)
	identityRepo := NewIdentityRepository(db)

	identity := &model.UserIdentity{IdentityKey: "fav-key", Nickname: "收藏者", Avatar: "a.png", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := identityRepo.Create(identity); err != nil {
		t.Fatalf("create identity: %v", err)
	}

	published := &model.Post{IdentityID: identity.ID, Content: "visible post", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	rejected := &model.Post{IdentityID: identity.ID, Content: "blocked post", Status: constants.PostStatusRejected, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := postRepo.Create(published); err != nil {
		t.Fatalf("create published post: %v", err)
	}
	if err := postRepo.Create(rejected); err != nil {
		t.Fatalf("create rejected post: %v", err)
	}

	// 首次收藏生效
	added, err := favRepo.AddIgnoreExists(&model.Favorite{IdentityID: identity.ID, PostID: published.ID, CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("add favorite: %v", err)
	}
	if !added {
		t.Fatal("first favorite should be inserted")
	}

	// 重复收藏不产生重复记录
	dup, err := favRepo.AddIgnoreExists(&model.Favorite{IdentityID: identity.ID, PostID: published.ID, CreatedAt: time.Now()})
	if err != nil {
		t.Fatalf("duplicate favorite: %v", err)
	}
	if dup {
		t.Fatal("duplicate favorite should be ignored")
	}
	count, err := favRepo.CountByPost(published.ID)
	if err != nil {
		t.Fatalf("count favorite: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	// 批量查询收藏状态
	got, err := favRepo.IsFavorited(identity.ID, []uint{published.ID, rejected.ID})
	if err != nil {
		t.Fatalf("is favorited: %v", err)
	}
	if !got[published.ID] || got[rejected.ID] {
		t.Fatalf("unexpected favorited map: %v", got)
	}

	// 收藏被屏蔽的帖子：关系保留
	if _, err := favRepo.AddIgnoreExists(&model.Favorite{IdentityID: identity.ID, PostID: rejected.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("favorite rejected post: %v", err)
	}
	favorites, total, err := favRepo.ListVisibleByIdentity(identity.ID, 1, 10)
	if err != nil {
		t.Fatalf("list visible favorites: %v", err)
	}
	if total != 1 || len(favorites) != 1 || favorites[0].PostID != published.ID {
		t.Fatalf("expected only visible post, got total=%d len=%d", total, len(favorites))
	}

	// 放行后（状态改为已发布）屏蔽帖自动重新出现，且原收藏时间保留
	blocked, err := postRepo.FindByID(rejected.ID)
	if err != nil {
		t.Fatalf("find blocked post: %v", err)
	}
	blocked.Status = constants.PostStatusPublished
	if err := postRepo.Update(blocked); err != nil {
		t.Fatalf("approve post: %v", err)
	}
	favorites, total, err = favRepo.ListVisibleByIdentity(identity.ID, 1, 10)
	if err != nil {
		t.Fatalf("list favorites after approve: %v", err)
	}
	if total != 2 || len(favorites) != 2 {
		t.Fatalf("expected 2 favorites after approve, got total=%d len=%d", total, len(favorites))
	}

	// 取消收藏
	deleted, err := favRepo.Delete(identity.ID, published.ID)
	if err != nil {
		t.Fatalf("delete favorite: %v", err)
	}
	if !deleted {
		t.Fatal("delete should affect one row")
	}
	if _, err := favRepo.Find(identity.ID, published.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFavoriteRepositoryIsolation(t *testing.T) {
	db := newTestDB(t)
	favRepo := NewFavoriteRepository(db)
	postRepo := NewPostRepository(db)
	identityRepo := NewIdentityRepository(db)

	a := &model.UserIdentity{IdentityKey: "iso-a", Nickname: "A", Avatar: "", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	b := &model.UserIdentity{IdentityKey: "iso-b", Nickname: "B", Avatar: "", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := identityRepo.Create(a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := identityRepo.Create(b); err != nil {
		t.Fatalf("create b: %v", err)
	}
	post := &model.Post{IdentityID: a.ID, Content: "shared", Status: constants.PostStatusPublished, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := postRepo.Create(post); err != nil {
		t.Fatalf("create post: %v", err)
	}

	// 两个身份各自收藏同一帖子，互不影响
	if _, err := favRepo.AddIgnoreExists(&model.Favorite{IdentityID: a.ID, PostID: post.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("a favorite: %v", err)
	}
	if _, err := favRepo.AddIgnoreExists(&model.Favorite{IdentityID: b.ID, PostID: post.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("b favorite: %v", err)
	}
	if count, _ := favRepo.CountByPost(post.ID); count != 2 {
		t.Fatalf("expected 2 favorites across identities, got %d", count)
	}
	if items, total, err := favRepo.ListVisibleByIdentity(a.ID, 1, 10); err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("identity A should see only its own favorite, got total=%d len=%d err=%v", total, len(items), err)
	}
	if items, _, _ := favRepo.ListVisibleByIdentity(b.ID, 1, 10); items[0].Post.IdentityID != a.ID {
		t.Fatal("identity B favorite should resolve the same post")
	}
}
