package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gbtreehole/backend/internal/model"
	"gorm.io/gorm"
)

type FavoriteRepository interface {
	Create(favorite *model.Favorite) error
	// Delete 删除收藏关系，返回实际删除的行数（0 表示原本就不存在）。
	Delete(identityID, postID uint) (int64, error)
	CountByPost(postID uint) (int64, error)
	IsFavorited(identityID uint, postIDs []uint) (map[uint]bool, error)
	// ListFavorites 返回某身份收藏中、目标帖子处于指定状态的收藏记录，按收藏时间倒序。
	ListFavorites(identityID uint, postStatus, page, pageSize int) ([]model.Favorite, int64, error)
	AdjustPostCount(postID uint, delta int) error
}

type favoriteRepository struct {
	db *gorm.DB
}

func NewFavoriteRepository(db *gorm.DB) FavoriteRepository {
	return &favoriteRepository{db: db}
}

// IsDuplicateErr 判断是否唯一索引冲突（MySQL 1062 / SQLite UNIQUE）。
func IsDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		strings.Contains(err.Error(), "Duplicate entry") ||
		strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (r *favoriteRepository) Create(favorite *model.Favorite) error {
	if err := r.db.Create(favorite).Error; err != nil {
		return fmt.Errorf("create favorite: %w", err)
	}
	return nil
}

func (r *favoriteRepository) Delete(identityID, postID uint) (int64, error) {
	tx := r.db.Where("identity_id = ? AND post_id = ?", identityID, postID).Delete(&model.Favorite{})
	if tx.Error != nil {
		return 0, fmt.Errorf("delete favorite: %w", tx.Error)
	}
	return tx.RowsAffected, nil
}

func (r *favoriteRepository) CountByPost(postID uint) (int64, error) {
	var count int64
	if err := r.db.Model(&model.Favorite{}).Where("post_id = ?", postID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count favorite: %w", err)
	}
	return count, nil
}

func (r *favoriteRepository) IsFavorited(identityID uint, postIDs []uint) (map[uint]bool, error) {
	result := make(map[uint]bool)
	if len(postIDs) == 0 {
		return result, nil
	}
	var favorites []model.Favorite
	if err := r.db.Where("identity_id = ? AND post_id IN ?", identityID, postIDs).Find(&favorites).Error; err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	for _, f := range favorites {
		result[f.PostID] = true
	}
	return result, nil
}

func (r *favoriteRepository) ListFavorites(identityID uint, postStatus, page, pageSize int) ([]model.Favorite, int64, error) {
	// JOIN posts 过滤审核屏蔽帖：屏蔽期间不展示，但收藏记录仍然保留，
	// 帖子放行（status 重新变为已发布）后会自动重新出现。
	base := func() *gorm.DB {
		return r.db.Model(&model.Favorite{}).
			Joins("JOIN posts ON posts.id = favorites.post_id").
			Where("favorites.identity_id = ? AND posts.status = ?", identityID, postStatus)
	}
	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count favorites: %w", err)
	}
	var favorites []model.Favorite
	if err := base().Order("favorites.created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&favorites).Error; err != nil {
		return nil, 0, fmt.Errorf("list favorites: %w", err)
	}
	return favorites, total, nil
}

func (r *favoriteRepository) AdjustPostCount(postID uint, delta int) error {
	expr := gorm.Expr("favorite_count + ?", delta)
	if delta < 0 {
		expr = gorm.Expr("CASE WHEN favorite_count > 0 THEN favorite_count - 1 ELSE 0 END")
	}
	if err := r.db.Model(&model.Post{}).Where("id = ?", postID).UpdateColumn("favorite_count", expr).Error; err != nil {
		return fmt.Errorf("adjust post favorite count: %w", err)
	}
	return nil
}
