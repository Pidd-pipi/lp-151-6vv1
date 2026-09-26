package dto

type FavoriteActionResponse struct {
	Favorited     bool  `json:"favorited"`
	FavoriteCount int64 `json:"favoriteCount"`
}
