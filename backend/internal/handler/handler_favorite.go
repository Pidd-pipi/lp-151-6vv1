package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gbtreehole/backend/internal/constants"
	"github.com/gbtreehole/backend/internal/dto"
	"github.com/gbtreehole/backend/internal/model"
	"github.com/gbtreehole/backend/internal/service"
)

type FavoriteHandler struct {
	favorites service.FavoriteService
	likes     service.LikeService
	logger    *slog.Logger
}

func NewFavoriteHandler(favorites service.FavoriteService, likes service.LikeService, logger *slog.Logger) *FavoriteHandler {
	return &FavoriteHandler{favorites: favorites, likes: likes, logger: logger}
}

// ToggleFavorite 收藏/取消收藏
// @Summary 收藏/取消收藏
// @Tags favorite
// @Accept json
// @Produce json
// @Param request body dto.FavoriteRequest true "收藏目标帖子"
// @Success 200 {object} Response
// @Router /api/v1/favorites/toggle [post]
func (h *FavoriteHandler) ToggleFavorite(c *gin.Context) {
	identityID := c.GetUint("identityId")
	var req dto.FavoriteRequest
	if !BindAndValidate(c, &req) {
		return
	}
	favorited, count, err := h.favorites.Toggle(identityID, req.PostID)
	if err != nil {
		if errors.Is(err, service.ErrPostNotFavoritable) {
			Fail(c, http.StatusNotFound, constants.CodeNotFound, "post not found")
			return
		}
		h.logger.Error("toggle favorite", "error", err)
		Fail(c, http.StatusInternalServerError, constants.CodeInternal, "toggle favorite failed")
		return
	}
	OK(c, dto.FavoriteResponse{Favorited: favorited, FavoriteCount: count})
}

// ListFavorites 我的收藏（按最近收藏时间倒序，屏蔽帖自动隐藏）
// @Summary 我的收藏
// @Tags favorite
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} Response
// @Router /api/v1/favorites [get]
func (h *FavoriteHandler) ListFavorites(c *gin.Context) {
	identityID := c.GetUint("identityId")
	var req dto.ListFavoriteRequest
	if !BindQuery(c, &req) {
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	favorites, total, err := h.favorites.ListMine(identityID, req.Page, req.PageSize)
	if err != nil {
		h.logger.Error("list favorites", "error", err)
		Fail(c, http.StatusInternalServerError, constants.CodeInternal, "list favorites failed")
		return
	}
	posts := make([]model.Post, 0, len(favorites))
	for _, f := range favorites {
		if f.Post != nil {
			posts = append(posts, *f.Post)
		}
	}
	ids := make([]uint, 0, len(posts))
	for _, p := range posts {
		ids = append(ids, p.ID)
	}
	likedMap := map[uint]bool{}
	if m, err := h.likes.IsLiked(identityID, "post", ids); err == nil {
		likedMap = m
	}
	// posts 保持收藏时间倒序，列表中的帖子对当前身份必然均为已收藏
	items := make([]dto.PostResponse, 0, len(posts))
	for i := range posts {
		resp := toPostResponse(&posts[i], likedMap[posts[i].ID])
		resp.Favorited = true
		items = append(items, resp)
	}
	OK(c, dto.PageResult{Items: items, Total: total, Page: req.Page, PageSize: req.PageSize})
}
