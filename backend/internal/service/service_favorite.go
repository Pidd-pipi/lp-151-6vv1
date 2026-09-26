package service

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
)

var ErrPostNotFavoritable = errors.New("post not found or cannot be favorited")

type FavoriteService interface {
	// Toggle 收藏/取消收藏。返回收藏后状态与服务端最新收藏数。
	// 重复收藏请求由唯一索引幂等处理，不会产生重复记录或计数漂移。
	Toggle(identityID uint, postID uint) (bool, int64, error)
	// IsFavorited 批量查询当前身份是否已收藏。
	IsFavorited(identityID uint, postIDs []uint) (map[uint]bool, error)
	// ListMine 按最近收藏时间倒序返回当前身份的可见收藏。
	ListMine(identityID uint, page, pageSize int) ([]model.Favorite, int64, error)
}

type favoriteService struct {
	favorites repository.FavoriteRepository
	posts     repository.PostRepository
	logger    *slog.Logger
}

func NewFavoriteService(favorites repository.FavoriteRepository, posts repository.PostRepository, logger *slog.Logger) FavoriteService {
	return &favoriteService{favorites: favorites, posts: posts, logger: logger}
}

func (s *favoriteService) Toggle(identityID uint, postID uint) (bool, int64, error) {
	// 收藏目标必须真实存在；被审核屏蔽的帖子仍允许取消收藏，
	// 但收藏关系本身不删除，放行后自动恢复展示。
	if _, err := s.posts.FindByID(postID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, 0, ErrPostNotFavoritable
		}
		return false, 0, fmt.Errorf("find post for favorite: %w", err)
	}

	existing, err := s.favorites.Find(identityID, postID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return false, 0, err
	}

	var favorited bool
	if existing != nil {
		// 已收藏 -> 取消收藏
		if _, err := s.favorites.Delete(identityID, postID); err != nil {
			return false, 0, err
		}
		favorited = false
	} else {
		// 未收藏 -> 收藏；唯一索引保证并发的重复插入只会生效一次，
		// 冲突时静默忽略并仍保持「已收藏」终态，不会误删对方刚写入的记录。
		if _, err := s.favorites.AddIgnoreExists(&model.Favorite{
			IdentityID: identityID,
			PostID:     postID,
			CreatedAt:  time.Now(),
		}); err != nil {
			return false, 0, err
		}
		favorited = true
	}

	// 收藏数始终以收藏关系表实时统计为准并回写冗余计数，
	// 保证按钮和列表数量与服务端一致，快速连点也不会重复计数。
	count, err := s.favorites.CountByPost(postID)
	if err != nil {
		return false, 0, err
	}
	if err := s.syncPostFavoriteCount(postID, int(count)); err != nil {
		// 冗余计数回写失败不影响收藏结果，仅记录日志
		s.logger.Error("sync post favorite count", "postId", postID, "error", err)
	}
	return favorited, count, nil
}

func (s *favoriteService) syncPostFavoriteCount(postID uint, count int) error {
	post, err := s.posts.FindByID(postID)
	if err != nil {
		return fmt.Errorf("find post: %w", err)
	}
	post.FavoriteCount = count
	return s.posts.Update(post)
}

func (s *favoriteService) IsFavorited(identityID uint, postIDs []uint) (map[uint]bool, error) {
	return s.favorites.IsFavorited(identityID, postIDs)
}

func (s *favoriteService) ListMine(identityID uint, page, pageSize int) ([]model.Favorite, int64, error) {
	return s.favorites.ListVisibleByIdentity(identityID, page, pageSize)
}
