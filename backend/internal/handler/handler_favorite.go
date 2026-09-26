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

// AddFavorite 收藏帖子（幂等，重复收藏不会产生重复记录、计数不会重复增加）
// @Summary 收藏帖子
// @Tags favorite
// @Produce json
// @Param id path int true "帖子ID"
// @Success 200 {object} Response
// @Router /api/v1/posts/{id}/favorite [post]
func (h *FavoriteHandler) AddFavorite(c *gin.Context) {
	identityID := c.GetUint("identityId")
	postID := parseID(c)
	if postID == 0 {
		return
	}
	favorited, count, err := h.favorites.Add(identityID, postID)
	if err != nil {
		if errors.Is(err, service.ErrPostNotFound) {
			Fail(c, http.StatusNotFound, constants.CodeNotFound, "post not found")
			return
		}
		h.logger.Error("add favorite", "error", err)
		Fail(c, http.StatusInternalServerError, constants.CodeInternal, "add favorite failed")
		return
	}
	OK(c, dto.FavoriteActionResponse{Favorited: favorited, FavoriteCount: count})
}

// RemoveFavorite 取消收藏（幂等）
// @Summary 取消收藏
// @Tags favorite
// @Produce json
// @Param id path int true "帖子ID"
// @Success 200 {object} Response
// @Router /api/v1/posts/{id}/favorite [delete]
func (h *FavoriteHandler) RemoveFavorite(c *gin.Context) {
	identityID := c.GetUint("identityId")
	postID := parseID(c)
	if postID == 0 {
		return
	}
	favorited, count, err := h.favorites.Remove(identityID, postID)
	if err != nil {
		h.logger.Error("remove favorite", "error", err)
		Fail(c, http.StatusInternalServerError, constants.CodeInternal, "remove favorite failed")
		return
	}
	OK(c, dto.FavoriteActionResponse{Favorited: favorited, FavoriteCount: count})
}

// ListFavorites 当前身份的收藏列表，按最近收藏时间倒序；
// 被审核屏蔽的帖子由服务端隐藏，放行后自动重新出现。
// @Summary 我的收藏
// @Tags favorite
// @Produce json
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} Response
// @Router /api/v1/favorites [get]
func (h *FavoriteHandler) ListFavorites(c *gin.Context) {
	identityID := c.GetUint("identityId")
	var req dto.ListPostRequest
	if !BindQuery(c, &req) {
		return
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	var posts []model.Post
	posts, total, err := h.favorites.List(identityID, req.Page, req.PageSize)
	if err != nil {
		h.logger.Error("list favorites", "error", err)
		Fail(c, http.StatusInternalServerError, constants.CodeInternal, "list favorites failed")
		return
	}
	items := buildPostResponses(h.likes, h.favorites, posts, identityID)
	OK(c, dto.PageResult{Items: items, Total: total, Page: req.Page, PageSize: req.PageSize})
}
