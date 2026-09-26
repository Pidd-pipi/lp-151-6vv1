package service

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/repository"
)

type FavoriteService interface {
	// Add 收藏帖子，重复收藏幂等：不产生重复记录、计数不重复增加。
	Add(identityID, postID uint) (favorited bool, count int64, err error)
	// Remove 取消收藏，重复取消幂等。
	Remove(identityID, postID uint) (favorited bool, count int64, err error)
	IsFavorited(identityID uint, postIDs []uint) (map[uint]bool, error)
	// List 返回收藏的帖子（按收藏时间倒序），已被审核屏蔽的帖子自动隐藏。
	List(identityID uint, page, pageSize int) ([]model.Post, int64, error)
}

type favoriteService struct {
	favorites repository.FavoriteRepository
	posts     repository.PostRepository
	logger    *slog.Logger
}

func NewFavoriteService(favorites repository.FavoriteRepository, posts repository.PostRepository, logger *slog.Logger) FavoriteService {
	return &favoriteService{favorites: favorites, posts: posts, logger: logger}
}

func (s *favoriteService) Add(identityID, postID uint) (bool, int64, error) {
	if _, err := s.posts.FindByID(postID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, 0, ErrPostNotFound
		}
		return false, 0, fmt.Errorf("check post exists: %w", err)
	}
	favorite := &model.Favorite{IdentityID: identityID, PostID: postID, CreatedAt: time.Now()}
	if err := s.favorites.Create(favorite); err != nil {
		// 唯一索引冲突意味着该身份已经收藏过：幂等返回当前状态，计数不变。
		if repository.IsDuplicateErr(err) {
			count, countErr := s.favorites.CountByPost(postID)
			if countErr != nil {
				return false, 0, countErr
			}
			return true, count, nil
		}
		return false, 0, err
	}
	if err := s.favorites.AdjustPostCount(postID, 1); err != nil {
		s.logger.Error("increment post favorite count", "postId", postID, "error", err)
	}
	count, err := s.favorites.CountByPost(postID)
	if err != nil {
		return false, 0, err
	}
	return true, count, nil
}

func (s *favoriteService) Remove(identityID, postID uint) (bool, int64, error) {
	// 以实际删除行数为准：并发/重复取消时只有一次会递减计数。
	affected, err := s.favorites.Delete(identityID, postID)
	if err != nil {
		return false, 0, err
	}
	if affected > 0 {
		if err := s.favorites.AdjustPostCount(postID, -1); err != nil {
			s.logger.Error("decrement post favorite count", "postId", postID, "error", err)
		}
	}
	count, err := s.favorites.CountByPost(postID)
	if err != nil {
		return false, 0, err
	}
	return false, count, nil
}

func (s *favoriteService) IsFavorited(identityID uint, postIDs []uint) (map[uint]bool, error) {
	return s.favorites.IsFavorited(identityID, postIDs)
}

func (s *favoriteService) List(identityID uint, page, pageSize int) ([]model.Post, int64, error) {
	items, total, err := s.favorites.ListFavorites(identityID, constants.PostStatusPublished, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	postIDs := make([]uint, 0, len(items))
	for _, item := range items {
		postIDs = append(postIDs, item.PostID)
	}
	posts, err := s.posts.ListByIDs(postIDs)
	if err != nil {
		return nil, 0, err
	}
	// ListByIDs 不保证顺序，按收藏记录的时间倒序重排。
	order := make(map[uint]int, len(postIDs))
	for i, id := range postIDs {
		order[id] = i
	}
	sort.Slice(posts, func(i, j int) bool {
		return order[posts[i].ID] < order[posts[j].ID]
	})
	return posts, total, nil
}
