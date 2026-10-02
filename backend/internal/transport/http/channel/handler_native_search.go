package channel

import (
	"net/http"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type NativeSearchResponse struct {
	Status   string `json:"status" enums:"enabled,unavailable,skipped"`
	Protocol string `json:"protocol"`
	ToolKey  string `json:"toolKey,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type NativeSearchResponseDoc struct {
	ErrorMsg string               `json:"errorMsg"`
	Data     NativeSearchResponse `json:"data"`
}

// ConfigureUserNativeSearch detects hosted search and enables it for an owned model.
// @Summary 检测并默认启用私有模型原生搜索
// @Tags User Upstreams
// @Security BearerAuth
// @Produce json
// @Param id path int true "上游 ID"
// @Param route_id path int true "路由 ID"
// @Success 200 {object} NativeSearchResponseDoc
// @Router /upstreams/{id}/models/{route_id}/native-search [post]
func (h *Handler) ConfigureUserNativeSearch(c *gin.Context) {
	h.configureNativeSearch(c, middleware.MustUserID(c))
}

// ConfigureAdminNativeSearch detects hosted search for a public single-source model.
// @Summary 检测并默认启用公共模型原生搜索
// @Tags Admin Channels
// @Security BearerAuth
// @Produce json
// @Param id path int true "上游 ID"
// @Param route_id path int true "路由 ID"
// @Success 200 {object} NativeSearchResponseDoc
// @Router /admin/llm/upstreams/{id}/models/{route_id}/native-search [post]
func (h *Handler) ConfigureAdminNativeSearch(c *gin.Context) {
	h.configureNativeSearch(c, 0)
}

func (h *Handler) configureNativeSearch(c *gin.Context, userID uint) {
	upstreamID, err := uintParam(c, "id")
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, errInvalidUpstreamID)
		return
	}
	routeID, err := uintParam(c, "route_id")
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, errInvalidRouteID)
		return
	}
	result, err := h.service.ConfigureNativeSearch(c.Request.Context(), userID, upstreamID, routeID)
	if err != nil {
		userUpstreamError(c, err)
		return
	}
	response.Success(c, NativeSearchResponse{Status: result.Status, Protocol: result.Protocol, ToolKey: result.ToolKey, Reason: result.Reason})
}
