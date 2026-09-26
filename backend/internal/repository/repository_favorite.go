package repository

import (
	"errors"
	"fmt"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FavoriteRepository interface {
	// AddIgnoreExists 插入收藏；(identity_id, post_id) 唯一索引兜底，
	// 重复插入（含快速连点的并发请求）不会产生重复记录。
	AddIgnoreExists(favorite *model.Favorite) (bool, error)
	Delete(identityID, postID uint) (bool, error)
	Find(identityID, postID uint) (*model.Favorite, error)
	CountByPost(postID uint) (int64, error)
	// IsFavorited 批量返回该身份是否已收藏指定帖子。
	IsFavorited(identityID uint, postIDs []uint) (map[uint]bool, error)
	// ListVisibleByIdentity 返回该身份的收藏关系（关联帖子已预加载），
	// 仅包含当前可见（status = published）的帖子；被审核屏蔽的收藏关系
	// 仍然保留在表中，只是不在结果里，放行后自动重新出现。
	// 按收藏时间倒序分页。
	ListVisibleByIdentity(identityID uint, page, pageSize int) ([]model.Favorite, int64, error)
}

type favoriteRepository struct {
	db *gorm.DB
}

// insertIgnoreExpression 返回「冲突时忽略插入」的子句。
// MySQL 与 SQLite 的忽略语法不同（INSERT IGNORE / INSERT OR IGNORE），
// 通过 clause.Insert.Modifier 注入，二者均不会因唯一键冲突而报错，
// 且 RowsAffected 在记录已存在时为 0。
var insertIgnoreExpression = func(db *gorm.DB) clause.Expression {
	if db.Dialector.Name() == "sqlite" {
		return clause.Insert{Modifier: "OR IGNORE"}
	}
	return clause.Insert{Modifier: "IGNORE"}
}

func NewFavoriteRepository(db *gorm.DB) FavoriteRepository {
	return &favoriteRepository{db: db}
}

func (r *favoriteRepository) AddIgnoreExists(favorite *model.Favorite) (bool, error) {
	result := r.db.Clauses(insertIgnoreExpression(r.db)).Create(favorite)
	if result.Error != nil {
		return false, fmt.Errorf("add favorite: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *favoriteRepository) Delete(identityID, postID uint) (bool, error) {
	result := r.db.Where("identity_id = ? AND post_id = ?", identityID, postID).Delete(&model.Favorite{})
	if result.Error != nil {
		return false, fmt.Errorf("delete favorite: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *favoriteRepository) Find(identityID, postID uint) (*model.Favorite, error) {
	var favorite model.Favorite
	if err := r.db.Where("identity_id = ? AND post_id = ?", identityID, postID).First(&favorite).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find favorite: %w", err)
	}
	return &favorite, nil
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

func (r *favoriteRepository) ListVisibleByIdentity(identityID uint, page, pageSize int) ([]model.Favorite, int64, error) {
	base := r.db.Model(&model.Favorite{}).
		Joins("JOIN posts ON posts.id = favorites.post_id").
		Where("favorites.identity_id = ? AND posts.status = ?", identityID, constants.PostStatusPublished)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count favorites: %w", err)
	}

	var favorites []model.Favorite
	if err := r.db.Preload("Post.Identity").Preload("Post.Tags").
		Joins("JOIN posts ON posts.id = favorites.post_id").
		Where("favorites.identity_id = ? AND posts.status = ?", identityID, constants.PostStatusPublished).
		Order("favorites.created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&favorites).Error; err != nil {
		return nil, 0, fmt.Errorf("list favorites: %w", err)
	}
	return favorites, total, nil
}
