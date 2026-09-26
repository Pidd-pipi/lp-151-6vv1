package model

import "time"

// Favorite 匿名身份对帖子的收藏关系。
// (identity_id, post_id) 唯一索引保证同一身份重复收藏只产生一条记录。
type Favorite struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	IdentityID uint      `gorm:"uniqueIndex:uniq_favorite_identity_post;not null" json:"identityId"`
	PostID     uint      `gorm:"uniqueIndex:uniq_favorite_identity_post;not null" json:"postId"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}
