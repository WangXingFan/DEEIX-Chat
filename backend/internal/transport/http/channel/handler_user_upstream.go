package channel

import (
	"errors"
	"net/http"

	appchannel "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/pagination"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

func userUpstreamError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appchannel.ErrUpstreamNotFound), errors.Is(err, appchannel.ErrModelAccessDenied), errors.Is(err, appchannel.ErrModelNotFound):
		response.ErrorFrom(c, http.StatusNotFound, errUpstreamNotFound)
	case errors.Is(err, appchannel.ErrUpstreamModelNotFound):
		response.ErrorFrom(c, http.StatusNotFound, errUpstreamModelNotFound)
	case errors.Is(err, appchannel.ErrInvalidUpstreamBaseURL), errors.Is(err, appchannel.ErrInvalidHeadersConfig), errors.Is(err, appchannel.ErrInvalidAPIKeysConfig), errors.Is(err, appchannel.ErrInvalidProtocolDefaultsConfig), errors.Is(err, appchannel.ErrInvalidJSONConfig), errors.Is(err, appchannel.ErrUserUpstreamLimit), errors.Is(err, appchannel.ErrUserModelLimit):
		response.ErrorFrom(c, http.StatusBadRequest, err)
	case errors.Is(err, appchannel.ErrNoActiveKey):
		response.ErrorFrom(c, http.StatusBadRequest, err)
	case errors.Is(err, appchannel.ErrRemoteModelsUnavailable):
		response.ErrorFrom(c, http.StatusBadGateway, err)
	default:
		response.InternalError(c)
	}
}

func (h *Handler) ListUserUpstreams(c *gin.Context) {
	page, pageSize := pagination.Parse(c.Query("page"), c.Query("page_size"))
	items, total, err := h.service.ListUserUpstreams(c.Request.Context(), middleware.MustUserID(c), page, pageSize, appchannel.ListUpstreamsInput{
		Query: c.Query("q"), Status: c.Query("status"), Compatible: c.Query("compatible"), Sort: c.Query("sort"),
	})
	if err != nil {
		userUpstreamError(c, err)
		return
	}
	results := make([]UpstreamResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toUpstreamResponse(item))
	}
	response.Success(c, struct {
		Total   int64              `json:"total"`
		Results []UpstreamResponse `json:"results"`
	}{Total: total, Results: results})
}

func (h *Handler) CreateUserUpstream(c *gin.Context) {
	var req CreateUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.CreateUserUpstream(c.Request.Context(), middleware.MustUserID(c), appchannel.CreateUpstreamInput{
		Name: req.Name, BaseURL: req.BaseURL, Compatible: req.Compatible, ProtocolDefaultsJSON: req.ProtocolDefaultsJSON, APIKeys: req.APIKeys, Status: req.Status,
		ConnectTimeoutMS: req.ConnectTimeoutMS, ReadTimeoutMS: req.ReadTimeoutMS, StreamIdleTimeoutMS: req.StreamIdleTimeoutMS,
		CbFailureThreshold: req.CbFailureThreshold, CbModelThreshold: req.CbModelThreshold, CbThresholdLogic: req.CbThresholdLogic,
		CbDurationMin: req.CbDurationMin, CbWindowMin: req.CbWindowMin, HeadersJSON: req.HeadersJSON,
	})
	if err != nil {
		userUpstreamError(c, err)
		return
	}
	response.Success(c, UpstreamDataResponse{Upstream: toUpstreamResponse(*item)})
}

func (h *Handler) UpdateUserUpstream(c *gin.Context) {
	upstreamID, err := uintParam(c, "id")
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, errInvalidUpstreamID)
		return
	}
	var req UpdateUpstreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.UpdateUserUpstream(c.Request.Context(), middleware.MustUserID(c), upstreamID, appchannel.UpdateUpstreamInput{
		Name: req.Name, BaseURL: req.BaseURL, Compatible: req.Compatible, ProtocolDefaultsJSON: req.ProtocolDefaultsJSON, APIKeys: req.APIKeys,
		AddAPIKeys: req.AddAPIKeys, DeleteAPIKeyIDs: req.DeleteAPIKeyIDs, Status: req.Status, ConnectTimeoutMS: req.ConnectTimeoutMS,
		ReadTimeoutMS: req.ReadTimeoutMS, StreamIdleTimeoutMS: req.StreamIdleTimeoutMS, CbFailureThreshold: req.CbFailureThreshold,
		CbModelThreshold: req.CbModelThreshold, CbThresholdLogic: req.CbThresholdLogic, CbDurationMin: req.CbDurationMin, CbWindowMin: req.CbWindowMin, HeadersJSON: req.HeadersJSON,
	})
	if err != nil {
		userUpstreamError(c, err)
		return
	}
	response.Success(c, UpstreamDataResponse{Upstream: toUpstreamResponse(*item)})
}

func (h *Handler) DeleteUserUpstream(c *gin.Context) {
	upstreamID, err := uintParam(c, "id")
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, errInvalidUpstreamID)
		return
	}
	if err := h.service.DeleteUserUpstream(c.Request.Context(), middleware.MustUserID(c), upstreamID); err != nil {
		userUpstreamError(c, err)
		return
	}
	response.Success(c, gin.H{})
}

func (h *Handler) ListUserRemoteModels(c *gin.Context) {
	upstreamID, err := uintParam(c, "id")
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, errInvalidUpstreamID)
		return
	}
	data, err := h.service.ListUserRemoteModels(c.Request.Context(), middleware.MustUserID(c), upstreamID)
	if err != nil {
		userUpstreamError(c, err)
		return
	}
	response.Success(c, toUpstreamRemoteModelsResponse(*data))
}

func (h *Handler) ListUserUpstreamModels(c *gin.Context) {
	upstreamID, err := uintParam(c, "id")
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, errInvalidUpstreamID)
		return
	}
	items, err := h.service.ListUserUpstreamModels(c.Request.Context(), middleware.MustUserID(c), upstreamID)
	if err != nil {
		userUpstreamError(c, err)
		return
	}
	results := make([]UpstreamModelResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toUpstreamModelResponse(item))
	}
	response.Success(c, gin.H{"items": results})
}

func (h *Handler) DeleteUserUpstreamModel(c *gin.Context) {
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
	if err := h.service.DeleteUserUpstreamModel(c.Request.Context(), middleware.MustUserID(c), upstreamID, routeID); err != nil {
		userUpstreamError(c, err)
		return
	}
	response.Success(c, gin.H{})
}

func (h *Handler) ImportUserModels(c *gin.Context) {
	upstreamID, err := uintParam(c, "id")
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, errInvalidUpstreamID)
		return
	}
	var req UserUpstreamModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	items, err := h.service.ImportUserModels(c.Request.Context(), middleware.MustUserID(c), upstreamID, req.ModelNames)
	if err != nil {
		userUpstreamError(c, err)
		return
	}
	results := make([]UpstreamModelResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toUpstreamModelResponse(item))
	}
	response.Success(c, gin.H{"items": results})
}
