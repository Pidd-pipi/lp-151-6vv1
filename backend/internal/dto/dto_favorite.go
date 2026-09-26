package dto

type FavoriteRequest struct {
	PostID uint `json:"postId" validate:"required"`
}

type FavoriteResponse struct {
	Favorited     bool  `json:"favorited"`
	FavoriteCount int64 `json:"favoriteCount"`
}

type ListFavoriteRequest struct {
	Page     int `json:"page" form:"page" validate:"omitempty,min=1"`
	PageSize int `json:"pageSize" form:"page_size" validate:"omitempty,min=1,max=100"`
}
