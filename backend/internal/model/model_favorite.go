package model

import "time"

// Favorite 匿名身份对帖子的收藏关系。
// (identity_id, post_id) 唯一索引保证重复点击不会产生重复记录，
// 审核屏蔽只改 posts.status，不删除收藏关系，放行后可自动重新出现。
type Favorite struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	IdentityID uint      `gorm:"uniqueIndex:idx_favorite_identity_post;not null" json:"identityId"`
	PostID     uint      `gorm:"uniqueIndex:idx_favorite_identity_post;index;not null" json:"postId"`
	Post       *Post     `gorm:"foreignKey:PostID" json:"post,omitempty"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}
