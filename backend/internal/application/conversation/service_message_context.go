package conversation

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	appchannel "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	appstorage "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/objectstorage"
	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/conv"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/textutil"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/objectstorage"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/apperr"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/tokenestimate"
)

const (
	MessageErrorCodeMediaImageStreamUnsupported   = "media.image_stream_unsupported"
	MessageErrorCodeKnowledgeBaseInvalidReference = "knowledge_base.invalid_reference"
	MessageErrorCodeKnowledgeBaseUnavailable      = "knowledge_base.unavailable"
	MessageErrorCodeKnowledgeBaseNotReady         = "knowledge_base.not_ready"
	MessageErrorCodeKnowledgeBaseScopeEmpty       = "knowledge_base.scope_empty"
	MessageErrorCodeUpstreamRateLimited           = "upstream.rate_limited"
	MessageErrorCodeUpstreamEmptyResponse         = "llm.empty_response"
	MessageErrorCodeUpstreamUnavailable           = "upstream.unavailable"
	MessageErrorCodeQuotaExceeded                 = "quota.exceeded"
	messageErrorCodeInternal                      = "internal.error"
)

const (
	maxConversationImageContextCount = 10
	maxConversationImageContextBytes = 20 * 1024 * 1024
	maxConversationImageSourceBytes  = 50 * 1024 * 1024
)

func normalizePublicID(raw string) string {
	return conv.NormalizePublicID(raw)
}

func isCJKRune(char rune) bool {
	return (char >= 0x2E80 && char <= 0x9FFF) ||
		(char >= 0xAC00 && char <= 0xD7AF) ||
		(char >= 0xF900 && char <= 0xFAFF) ||
		(char >= 0x20000 && char <= 0x2A6DF)
}

func estimateContentPartTokens(part llm.ContentPart) int64 {
	switch part.Kind {
	case llm.ContentPartImage:
		return 255
	case llm.ContentPartFile:
		return tokenestimate.Estimate(part.FileName) + tokenestimate.Estimate(part.Text) + 8
	default:
		return tokenestimate.Estimate(part.Text)
	}
}

func estimateMessageTokens(message llm.Message) int64 {
	var tokens int64 = 4
	if message.Role != "" {
		tokens += 1
	}
	if len(message.Parts) > 0 {
		for _, part := range message.Parts {
			tokens += estimateContentPartTokens(part)
		}
	} else {
		tokens += tokenestimate.Estimate(message.Content)
	}
	tokens += tokenestimate.Estimate(message.ReasoningContent)
	for _, call := range message.ToolCalls {
		tokens += tokenestimate.Estimate(call.ToolCallID)
		tokens += tokenestimate.Estimate(call.ToolName)
		tokens += tokenestimate.Estimate(call.ArgumentsJSON)
		tokens += 8
	}
	for _, result := range message.ToolResults {
		tokens += tokenestimate.Estimate(result.ToolCallID)
		tokens += tokenestimate.Estimate(result.ToolName)
		tokens += tokenestimate.Estimate(result.OutputJSON)
		tokens += tokenestimate.Estimate(result.Error)
		tokens += 8
	}
	return tokens
}

func estimatePromptTokens(messages []llm.Message) int64 {
	var tokens int64 = 2
	for _, message := range messages {
		tokens += estimateMessageTokens(message)
	}
	if tokens < 0 {
		return 0
	}
	return tokens
}

func buildContextPolicyJSON(cfg config.Config) string {
	policy := map[string]any{
		"max_turns":                      cfg.ContextMaxTurns,
		"compact_enabled":                cfg.ContextCompactEnabled,
		"context_window_fallback_tokens": cfg.ContextWindowFallbackTokens,
		"compact_trigger_percent":        cfg.ContextCompactTriggerPercent,
		"compact_preserve_recent_turns":  cfg.ContextCompactPreserve,
	}
	raw, err := json.Marshal(policy)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func inferProvider(platformModelName string) string {
	code := strings.ToLower(strings.TrimSpace(platformModelName))
	switch {
	case strings.HasPrefix(code, "gpt-"):
		return "openai"
	case strings.HasPrefix(code, "claude-"):
		return "anthropic"
	default:
		return "internal"
	}
}

func classifyRunErrorCode(err error) string {
	if errors.Is(err, ErrGeneratedMediaArtifactUnavailable) {
		return MessageErrorCodeMediaArtifactUnavailable
	}
	var upstreamErr *llm.UpstreamError
	if errors.As(err, &upstreamErr) && isImageStreamConfigurationFailure(upstreamErr) {
		return MessageErrorCodeMediaImageStreamUnsupported
	}
	if IsUpstreamRateLimitError(err) {
		return MessageErrorCodeUpstreamRateLimited
	}
	if errors.Is(err, ErrUpstreamEmptyResponse) {
		return MessageErrorCodeUpstreamEmptyResponse
	}
	if errors.Is(err, ErrUpstreamRequestFailed) {
		return MessageErrorCodeUpstreamUnavailable
	}
	if code := apperr.Code(err); code != "" {
		return code
	}
	return messageErrorCodeInternal
}

// IsUpstreamRateLimitError 判断错误是否来自真实上游 429 或本地路由级退避。
func IsUpstreamRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, appchannel.ErrAllRoutesRateLimited) {
		return true
	}
	var upstreamErr *llm.UpstreamError
	return errors.As(err, &upstreamErr) && upstreamErr.StatusCode == 429
}

func messageErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	var upstreamErr *llm.UpstreamError
	if errors.As(err, &upstreamErr) {
		return upstreamErrorSummary(upstreamErr)
	}
	if errors.Is(err, ErrUpstreamRequestFailed) {
		return "upstream service unavailable"
	}
	if coded, ok := apperr.Find(err); ok {
		if message := strings.TrimSpace(coded.Message()); message != "" {
			return message
		}
	}
	return "internal server error"
}

func isMessageGenerationCanceledError(err error) bool {
	return errors.Is(err, ErrMessageGenerationCanceled)
}

func messageErrorDebug(err error) *llm.UpstreamDebugSnapshot {
	if err == nil {
		return nil
	}
	var upstreamErr *llm.UpstreamError
	if errors.As(err, &upstreamErr) {
		return sanitizeUpstreamDebugSnapshot(upstreamErr.Debug)
	}
	return nil
}

func sanitizeUpstreamDebugSnapshot(debug *llm.UpstreamDebugSnapshot) *llm.UpstreamDebugSnapshot {
	if debug == nil {
		return nil
	}
	return &llm.UpstreamDebugSnapshot{
		Request: llm.UpstreamDebugRequest{
			Method:        debug.Request.Method,
			Path:          debug.Request.Path,
			Body:          sanitizeUpstreamNameJSON(llm.SanitizeUpstreamDebugBody(debug.Request.Body)),
			BodyBytes:     debug.Request.BodyBytes,
			BodyTruncated: debug.Request.BodyTruncated,
			RedactedParts: debug.Request.RedactedParts,
		},
		Response: llm.UpstreamDebugResponse{
			StatusCode:    debug.Response.StatusCode,
			Body:          sanitizeUpstreamNameJSON(llm.SanitizeUpstreamDebugBody(debug.Response.Body)),
			BodyBytes:     debug.Response.BodyBytes,
			BodyTruncated: debug.Response.BodyTruncated,
			RedactedParts: debug.Response.RedactedParts,
		},
	}
}

func sanitizeUpstreamNameJSON(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return raw
	}
	var payload any
	if err := json.Unmarshal([]byte(value), &payload); err != nil {
		return raw
	}
	deleteUpstreamNameValues(payload, "")
	data, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return string(data)
}

func deleteUpstreamNameValues(value any, parentKey string) {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if isUpstreamNameKey(key, parentKey) {
				delete(current, key)
				continue
			}
			deleteUpstreamNameValues(child, key)
		}
	case []any:
		for _, child := range current {
			deleteUpstreamNameValues(child, parentKey)
		}
	}
}

func isUpstreamNameKey(key string, parentKey string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "_", ""))
	if normalized == "upstreamname" {
		return true
	}
	return strings.ToLower(strings.TrimSpace(parentKey)) == "upstream" && (normalized == "name" || normalized == "displayname")
}

func upstreamErrorSummary(err *llm.UpstreamError) string {
	if err == nil {
		return ""
	}
	lines := make([]string, 0, 3)
	hint := imageStreamConfigurationHint(err)
	if isSuccessfulUpstreamStatus(err.StatusCode) {
		lines = append(lines, fmt.Sprintf("模型响应格式不兼容（HTTP %d）", err.StatusCode))
		lines = append(lines, "错误：上游返回成功状态码，但响应格式与当前协议不兼容")
		if hint != "" {
			lines = append(lines, hint)
		}
		return strings.Join(lines, "\n")
	}
	if err.StatusCode > 0 {
		lines = append(lines, fmt.Sprintf("模型请求失败（HTTP %d）", err.StatusCode))
	} else {
		lines = append(lines, "模型请求失败")
	}
	if message := normalizeUpstreamErrorMessage(err.Message); message != "" {
		lines = append(lines, "错误："+message)
	}
	if hint != "" {
		lines = append(lines, hint)
	}
	return strings.Join(lines, "\n")
}

func isSuccessfulUpstreamStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func normalizeUpstreamErrorMessage(message string) string {
	value := strings.TrimSpace(message)
	if value == "" || looksLikeRawSSEBody(value) {
		return ""
	}
	return value
}

func looksLikeRawSSEBody(value string) bool {
	normalized := strings.TrimSpace(value)
	return strings.HasPrefix(normalized, "data:") ||
		strings.Contains(normalized, "\ndata:") ||
		strings.Contains(normalized, " data:")
}

func imageStreamConfigurationHint(err *llm.UpstreamError) string {
	if !isImageStreamConfigurationFailure(err) {
		return ""
	}
	return "Tips：当前上游可能不支持流式响应，请管理员在模型能力中关闭“图像流式调用”（设置 image.stream=false）后重试。"
}

func isImageStreamConfigurationFailure(err *llm.UpstreamError) bool {
	return err != nil && isImageStreamingUpstreamRequest(err.Debug) && isStreamingResponseFormatFailure(err)
}

func isImageStreamingUpstreamRequest(debug *llm.UpstreamDebugSnapshot) bool {
	if debug == nil {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(debug.Request.Path))
	body := strings.TrimSpace(debug.Request.Body)
	if strings.Contains(path, "/images/") {
		return jsonObjectFieldIsTrue(body, "stream")
	}
	if strings.Contains(path, ":streamgeneratecontent") {
		return geminiImageResponseRequested(body)
	}
	return false
}

func jsonObjectFieldIsTrue(raw string, key string) bool {
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return false
	}
	switch value := payload[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func geminiImageResponseRequested(raw string) bool {
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return false
	}
	config, ok := payload["generationConfig"].(map[string]any)
	if !ok {
		return false
	}
	return containsImageModality(config["responseModalities"])
}

func containsImageModality(raw any) bool {
	switch value := raw.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "image")
	case []any:
		for _, item := range value {
			if containsImageModality(item) {
				return true
			}
		}
	case []string:
		for _, item := range value {
			if containsImageModality(item) {
				return true
			}
		}
	}
	return false
}

func isStreamingResponseFormatFailure(err *llm.UpstreamError) bool {
	detail := strings.ToLower(strings.TrimSpace(err.Message + "\n" + err.Body))
	if err.Debug != nil {
		detail = strings.TrimSpace(detail + "\n" + strings.ToLower(err.Debug.Response.Body))
	}
	if detail == "" {
		return false
	}
	if strings.Contains(detail, "invalid character") && strings.Contains(detail, "looking for beginning of value") {
		return true
	}
	return isStreamUnsupportedError(err)
}

func wrapUpstreamRequestError(cause error) error {
	if cause == nil {
		return ErrUpstreamRequestFailed
	}
	return fmt.Errorf("%w: %w", ErrUpstreamRequestFailed, cause)
}

func mapRouteResolutionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, appchannel.ErrModelAccessDenied):
		return ErrModelAccessDenied
	case errors.Is(err, appchannel.ErrRouteNotFound), errors.Is(err, appchannel.ErrModelNotFound):
		return ErrModelRouteNotConfigured
	case errors.Is(err, appchannel.ErrAllRoutesUnavailable), errors.Is(err, appchannel.ErrAllRoutesRateLimited):
		return wrapUpstreamRequestError(err)
	default:
		return err
	}
}

// MessageErrorCode 返回适合边界层和前端本地化使用的稳定错误码。
func MessageErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrGeneratedMediaArtifactUnavailable) {
		return MessageErrorCodeMediaArtifactUnavailable
	}
	var upstreamErr *llm.UpstreamError
	if errors.As(err, &upstreamErr) && isImageStreamConfigurationFailure(upstreamErr) {
		return MessageErrorCodeMediaImageStreamUnsupported
	}
	if IsUpstreamRateLimitError(err) {
		return MessageErrorCodeUpstreamRateLimited
	}
	if errors.Is(err, ErrUpstreamEmptyResponse) {
		return MessageErrorCodeUpstreamEmptyResponse
	}
	return ""
}

// MessageErrorDebug 返回脱敏后的上游请求/响应快照，用于排查兼容性问题。
func MessageErrorDebug(err error) *llm.UpstreamDebugSnapshot {
	return messageErrorDebug(err)
}

func normalizeAttachmentKind(kind string, mimeType string) string {
	value := strings.TrimSpace(kind)
	if value != "" {
		return value
	}
	return inferAttachmentKind(mimeType)
}

// NormalizeAttachmentKind 规范化附件类型，供边界层复用。
func NormalizeAttachmentKind(kind string, mimeType string) string {
	return normalizeAttachmentKind(kind, mimeType)
}

func inferAttachmentKind(mimeType string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/") {
		return "image"
	}
	return "file"
}

func normalizeBranchReason(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "retry", "edit":
		return value
	default:
		return "default"
	}
}

func normalizeMessageFeedback(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "up":
		return "up"
	case "down":
		return "down"
	default:
		return ""
	}
}

func fallbackContentType(contentType string) string {
	value := strings.TrimSpace(contentType)
	if value == "" {
		return "text"
	}
	return value
}

func shouldFallbackToNonStreaming(err error) bool {
	var upstreamErr *llm.UpstreamError
	if !errors.As(err, &upstreamErr) {
		return false
	}
	switch upstreamErr.StatusCode {
	case 405, 415, 501:
		return true
	default:
		return isStreamUnsupportedError(upstreamErr)
	}
}

type generationAttemptObservation struct {
	emitted bool
}

func (o *generationAttemptObservation) markObservable() {
	if o != nil {
		o.emitted = true
	}
}

func (o *generationAttemptObservation) canRetry(err error, classify func(error) bool) bool {
	return o != nil && err != nil && !o.emitted && classify != nil && classify(err)
}

func isStreamUnsupportedError(err *llm.UpstreamError) bool {
	detail := strings.ToLower(strings.TrimSpace(err.Message + " " + err.Body))
	if detail == "" || !strings.Contains(detail, "stream") {
		return false
	}
	for _, marker := range []string{
		"not support",
		"not_supported",
		"unsupported",
		"not available",
		"does not support",
		"doesn't support",
	} {
		if strings.Contains(detail, marker) {
			return true
		}
	}
	return false
}

type userContextInput struct {
	Attachments         []AttachmentInput
	RAGChunks           []domainconversation.RAGChunk
	RAGNotice           string
	HistoricalArtifacts []domainconversation.ContextArtifact
	CurrentArtifacts    []domainconversation.ContextArtifact
	Snapshot            *snapshotContext
}

type snapshotContext struct {
	Summary  string
	FromTurn int
	ToTurn   int
	Strategy string
}

// prependStableFileContext 将可全文注入的文本文件固定放在消息前缀，避免多轮对话中
// 同一份文件内容漂移到最新 user 消息，破坏上游前缀缓存。
func prependStableFileContext(messages []llm.Message, attachments []AttachmentInput) []llm.Message {
	contextXML := buildStableFileContextXML(attachments)
	if contextXML.empty() {
		return messages
	}
	content := buildUserContextPrompt("", contextXML)
	if strings.TrimSpace(content) == "" {
		return messages
	}
	result := make([]llm.Message, 0, len(messages)+1)
	result = append(result, llm.Message{
		Role:    "system",
		Content: content,
	})
	result = append(result, messages...)
	return result
}

func buildStableFileContextXML(attachments []AttachmentInput) userContextXML {
	if len(attachments) == 0 {
		return userContextXML{}
	}
	items := make([]AttachmentInput, 0, len(attachments))
	for _, att := range attachments {
		if !isStableTextAttachment(att) {
			continue
		}
		items = append(items, att)
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := stableAttachmentSortKey(items[i])
		right := stableAttachmentSortKey(items[j])
		return left < right
	})

	contextXML := userContextXML{files: make([]string, 0, len(items))}
	for _, att := range items {
		contextXML.files = append(contextXML.files, formatAttachmentFileContext(att.FileName, att.ExtractedText))
	}
	return contextXML
}

func stableAttachmentSortKey(att AttachmentInput) string {
	if value := strings.TrimSpace(att.FileID); value != "" {
		return "0:" + value
	}
	if value := strings.TrimSpace(att.SHA256); value != "" {
		return "1:" + value
	}
	if value := strings.TrimSpace(att.FileName); value != "" {
		return "2:" + value
	}
	return "3:"
}

type conversationImageRef struct {
	messageIndex int
	fileID       string
}

func conversationImageRefs(messages []domainconversation.Message, attachments []AttachmentInput, limit int) []conversationImageRef {
	if len(messages) == 0 || limit <= 0 {
		return nil
	}
	available := make(map[string]AttachmentInput, len(attachments))
	current := make(map[string]struct{})
	for _, att := range attachments {
		fileID := strings.TrimSpace(att.FileID)
		if fileID == "" {
			continue
		}
		if att.Current {
			current[fileID] = struct{}{}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(att.ContextMode), fileContextModeDirectImage) &&
			normalizeAttachmentKind(att.Kind, textutil.FirstNonEmpty(att.DetectedMIME, att.MimeType)) == "image" {
			available[fileID] = att
		}
	}

	refs := make([]conversationImageRef, 0)
	historyIndex := 0
	for _, message := range messages {
		if message.Role != "user" && message.Role != "assistant" && message.Role != "system" {
			continue
		}
		if message.Role == "user" {
			for _, snapshot := range parseAttachmentSnapshotRefs(message.Attachments) {
				fileID := strings.TrimSpace(snapshot.FileID)
				if fileID == "" {
					continue
				}
				if _, isCurrent := current[fileID]; isCurrent {
					continue
				}
				_, isAvailable := available[fileID]
				snapshotMIME := textutil.FirstNonEmpty(snapshot.DetectedMIME, snapshot.MimeType)
				if !isAvailable && normalizeAttachmentKind(snapshot.Kind, snapshotMIME) != "image" {
					continue
				}
				refs = append(refs, conversationImageRef{messageIndex: historyIndex, fileID: fileID})
			}
		}
		historyIndex++
	}
	if len(refs) > limit {
		refs = refs[len(refs)-limit:]
	}
	return refs
}

func (s *Service) injectConversationImageContext(
	ctx context.Context,
	messages []llm.Message,
	domainMessages []domainconversation.Message,
	attachments []AttachmentInput,
	cfg config.Config,
) ([]llm.Message, error) {
	refs := conversationImageRefs(domainMessages, attachments, maxConversationImageContextCount)
	if len(refs) == 0 {
		return messages, nil
	}

	attachmentByFileID := make(map[string]AttachmentInput, len(attachments))
	for _, att := range attachments {
		attachmentByFileID[strings.TrimSpace(att.FileID)] = att
	}
	maxDim := cfg.ImageMaxDimension
	if maxDim <= 0 {
		maxDim = 1024
	}
	cache := s.imageContextCache
	if cache == nil {
		cache = defaultPreparedConversationImageCache()
	}
	storeProvider := s.storeProvider

	var store objectstorage.Store
	partsByRef := make(map[int]llm.ContentPart, len(refs))
	loadedByFileID := make(map[string]llm.ContentPart, len(refs))
	totalBytes := 0
	for index := len(refs) - 1; index >= 0; index-- {
		ref := refs[index]
		part, loaded := loadedByFileID[ref.fileID]
		if !loaded {
			att, ok := attachmentByFileID[ref.fileID]
			if !ok || strings.TrimSpace(att.StoragePath) == "" {
				return nil, fmt.Errorf("%w: historical image %s", ErrInvalidFileReference, ref.fileID)
			}
			mime := resolveImageMimeType(textutil.FirstNonEmpty(att.DetectedMIME, att.MimeType))
			cacheKey := preparedConversationImageCacheKey(att, maxDim, mime)
			if cached, ok := cache.get(cacheKey); ok {
				part = llm.ContentPart{Kind: llm.ContentPartImage, MimeType: cached.mimeType, Data: cached.data}
			} else {
				if store == nil {
					if storeProvider == nil {
						return nil, fmt.Errorf("%w: open object storage: %w", ErrFileNotFound, appstorage.ErrProviderNotConfigured)
					}
					openedStore, openErr := storeProvider.Open(ctx)
					if openErr != nil {
						return nil, fmt.Errorf("%w: open object storage: %w", ErrFileNotFound, openErr)
					}
					store = openedStore
				}
				reader, _, openErr := store.Open(ctx, strings.TrimSpace(att.StoragePath))
				if openErr != nil {
					return nil, fmt.Errorf("%w: historical image %s: %w", ErrFileNotFound, ref.fileID, openErr)
				}
				data, readErr := io.ReadAll(io.LimitReader(reader, maxConversationImageSourceBytes+1))
				closeErr := reader.Close()
				if readErr != nil {
					return nil, fmt.Errorf("%w: read historical image %s: %w", ErrFileNotFound, ref.fileID, readErr)
				}
				if closeErr != nil {
					return nil, fmt.Errorf("%w: close historical image %s: %w", ErrFileNotFound, ref.fileID, closeErr)
				}
				if len(data) == 0 {
					return nil, fmt.Errorf("%w: historical image %s is empty", ErrInvalidFileReference, ref.fileID)
				}
				if len(data) > maxConversationImageSourceBytes {
					return nil, fmt.Errorf("%w: historical image %s exceeds source limit", ErrFileTooLarge, ref.fileID)
				}
				resized, actualMIME := resizeImageIfNeeded(data, mime, maxDim)
				part = llm.ContentPart{Kind: llm.ContentPartImage, MimeType: actualMIME, Data: resized}
				cache.put(cacheKey, preparedConversationImage{data: resized, mimeType: actualMIME})
			}
			loadedByFileID[ref.fileID] = part
		}
		if len(part.Data) == 0 {
			return nil, fmt.Errorf("%w: historical image %s is empty", ErrInvalidFileReference, ref.fileID)
		}
		if totalBytes+len(part.Data) > maxConversationImageContextBytes {
			return nil, fmt.Errorf("%w: historical image context exceeds %d bytes", ErrFileTooLarge, maxConversationImageContextBytes)
		}
		totalBytes += len(part.Data)
		partsByRef[index] = part
	}

	result := cloneLLMMessages(messages)
	for index, ref := range refs {
		part := partsByRef[index]
		if ref.messageIndex < 0 || ref.messageIndex >= len(result) {
			return nil, fmt.Errorf("%w: historical image message index", ErrInvalidFileReference)
		}
		message := result[ref.messageIndex]
		message.Parts = append([]llm.ContentPart(nil), message.Parts...)
		if len(message.Parts) == 0 && strings.TrimSpace(message.Content) != "" {
			message.Parts = append(message.Parts, llm.ContentPart{Kind: llm.ContentPartText, Text: message.Content})
			message.Content = ""
		}
		message.Parts = append(message.Parts, part)
		result[ref.messageIndex] = message
	}
	return result, nil
}

func imageAttachmentsForCurrentUser(attachments []AttachmentInput) []AttachmentInput {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]AttachmentInput, 0)
	for _, att := range attachments {
		if att.Current && normalizeAttachmentKind(att.Kind, att.MimeType) == "image" {
			result = append(result, att)
		}
	}
	return result
}

func injectUserContext(
	ctx context.Context,
	messages []llm.Message,
	input userContextInput,
	cfg config.Config,
	storeProvider appstorage.Provider,
) []llm.Message {
	if len(input.Attachments) == 0 &&
		len(input.RAGChunks) == 0 &&
		strings.TrimSpace(input.RAGNotice) == "" &&
		len(input.HistoricalArtifacts) == 0 &&
		input.Snapshot == nil {
		return messages
	}

	maxDim := cfg.ImageMaxDimension
	if maxDim <= 0 {
		maxDim = 1024
	}

	// 找到最后一条用户消息，构建 ContentParts
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		return messages
	}

	lastUserMsg := messages[lastUserIdx]
	imageParts := make([]llm.ContentPart, 0, len(lastUserMsg.Parts)+len(input.Attachments))
	for _, part := range lastUserMsg.Parts {
		if part.Kind == llm.ContentPartImage && len(part.Data) > 0 {
			imageParts = append(imageParts, part)
		}
	}
	contextXML := buildUserContextXML(input)

	for _, att := range input.Attachments {
		kind := normalizeAttachmentKind(att.Kind, att.MimeType)
		if kind == "image" {
			// 图片：读取文件字节并缩放
			storagePath := strings.TrimSpace(att.StoragePath)
			if storagePath == "" {
				continue
			}
			if storeProvider == nil {
				continue
			}
			store, storeErr := storeProvider.Open(ctx)
			if storeErr != nil {
				continue
			}
			reader, _, readErr := store.Open(ctx, storagePath)
			if readErr != nil {
				continue
			}
			imgData, readErr := io.ReadAll(io.LimitReader(reader, 50*1024*1024))
			_ = reader.Close()
			if readErr != nil {
				continue
			}
			mime := resolveImageMimeType(att.MimeType)
			resized, actualMIME := resizeImageIfNeeded(imgData, mime, maxDim)
			imageParts = append(imageParts, llm.ContentPart{
				Kind:     llm.ContentPartImage,
				MimeType: actualMIME,
				Data:     resized,
			})
		}
	}

	if len(imageParts) == 0 && contextXML.empty() {
		return messages
	}

	content := strings.TrimSpace(userMessageText(lastUserMsg))
	if !contextXML.empty() {
		content = buildUserContextPrompt(content, contextXML)
	}

	result := make([]llm.Message, len(messages))
	copy(result, messages)
	if len(imageParts) == 0 {
		result[lastUserIdx] = llm.Message{
			Role:    lastUserMsg.Role,
			Content: content,
		}
		return result
	}

	parts := make([]llm.ContentPart, 0, 1+len(imageParts))
	if content != "" {
		parts = append(parts, llm.ContentPart{
			Kind: llm.ContentPartText,
			Text: content,
		})
	}
	parts = append(parts, imageParts...)
	result[lastUserIdx] = llm.Message{Role: lastUserMsg.Role, Parts: parts}
	return result
}

func userMessageText(message llm.Message) string {
	if strings.TrimSpace(message.Content) != "" || len(message.Parts) == 0 {
		return message.Content
	}
	var builder strings.Builder
	for _, part := range message.Parts {
		if part.Kind != llm.ContentPartText && part.Kind != llm.ContentPartFile {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(part.Text)
	}
	return builder.String()
}

func formatAttachmentFileContext(fileName string, text string) string {
	name := strings.TrimSpace(fileName)
	if name == "" {
		name = "未命名文件"
	}
	return `<file name="` + xmlEscapeAttr(name) + `">` + xmlEscapeText(strings.TrimSpace(text)) + `</file>`
}

type userContextXML struct {
	summary   string
	files     []string
	evidence  []string
	rag       []string
	ragNotice string
}

func (x userContextXML) empty() bool {
	return strings.TrimSpace(x.summary) == "" &&
		len(x.files) == 0 &&
		len(x.evidence) == 0 &&
		len(x.rag) == 0 &&
		strings.TrimSpace(x.ragNotice) == ""
}

func buildUserContextXML(input userContextInput) userContextXML {
	return userContextXML{
		summary:   formatSnapshotContext(input.Snapshot),
		evidence:  formatHistoricalEvidenceContext(input.HistoricalArtifacts),
		rag:       formatRAGFileContext(input.RAGChunks),
		ragNotice: strings.TrimSpace(input.RAGNotice),
	}
}

func formatSnapshotContext(snapshot *snapshotContext) string {
	if snapshot == nil || strings.TrimSpace(snapshot.Summary) == "" {
		return ""
	}
	attrs := ` from="` + xmlEscapeAttr(fmt.Sprintf("%d", snapshot.FromTurn)) + `" to="` + xmlEscapeAttr(fmt.Sprintf("%d", snapshot.ToTurn)) + `"`
	if strategy := strings.TrimSpace(snapshot.Strategy); strategy != "" {
		attrs += ` strategy="` + xmlEscapeAttr(strategy) + `"`
	}
	return "<sum" + attrs + ">" + xmlEscapeText(strings.TrimSpace(snapshot.Summary)) + "</sum>"
}

func formatRAGFileContext(chunks []domainconversation.RAGChunk) []string {
	if len(chunks) == 0 {
		return nil
	}
	items := make([]string, 0, len(chunks))
	for index, chunk := range chunks {
		text := strings.TrimSpace(chunk.Content)
		if text == "" {
			continue
		}
		name := strings.TrimSpace(chunk.FileName)
		if name == "" {
			name = strings.TrimSpace(chunk.FileID)
		}
		if name == "" {
			name = "unknown"
		}
		chunkIndex := chunk.ChunkIndex
		if chunkIndex <= 0 {
			chunkIndex = index + 1
		}
		attrs := `name="` + xmlEscapeAttr(name) + `" i="` + xmlEscapeAttr(fmt.Sprintf("%d", chunkIndex)) + `"`
		if chunk.Modality == domainconversation.FileChunkModalityImage {
			// 图片分片的文本来自 OCR；原图已作为附件随消息发送。
			attrs += ` kind="image"`
		}
		items = append(items, `<doc `+attrs+`>`+xmlEscapeText(text)+`</doc>`)
	}
	return items
}

func formatHistoricalEvidenceContext(artifacts []domainconversation.ContextArtifact) []string {
	if len(artifacts) == 0 {
		return nil
	}
	items := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		content := strings.TrimSpace(artifact.Content)
		if content == "" {
			continue
		}
		kind := strings.TrimSpace(string(artifact.Kind))
		if kind == "" {
			kind = "evidence"
		}
		source := strings.TrimSpace(artifact.SourceTitle)
		if source == "" {
			source = strings.TrimSpace(artifact.SourceID)
		}
		if source == "" {
			source = "unknown"
		}
		items = append(items, `<ev k="`+xmlEscapeAttr(kind)+`" src="`+xmlEscapeAttr(source)+`">`+xmlEscapeText(textutil.CompactSnippet(content, 500))+`</ev>`)
	}
	return items
}

func buildUserContextPrompt(userRequest string, contextXML userContextXML) string {
	var builder strings.Builder
	builder.WriteString("<ctx>")
	if strings.TrimSpace(contextXML.summary) != "" {
		builder.WriteString("\n")
		builder.WriteString(contextXML.summary)
	}
	if len(contextXML.files) > 0 {
		builder.WriteString("\n<files>\n")
		builder.WriteString(strings.Join(contextXML.files, "\n"))
		builder.WriteString("\n</files>")
	}
	if len(contextXML.evidence) > 0 {
		builder.WriteString("\n<evs>\n")
		builder.WriteString(strings.Join(contextXML.evidence, "\n"))
		builder.WriteString("\n</evs>")
	}
	if len(contextXML.rag) > 0 {
		builder.WriteString("\n<rag>\n")
		builder.WriteString(strings.Join(contextXML.rag, "\n"))
		builder.WriteString("\n</rag>")
	}
	if strings.TrimSpace(contextXML.ragNotice) != "" {
		builder.WriteString("\n<rag_status>")
		builder.WriteString(xmlEscapeText(contextXML.ragNotice))
		builder.WriteString("</rag_status>")
	}
	builder.WriteString("\n</ctx>")

	request := strings.TrimSpace(userRequest)
	if request != "" {
		builder.WriteString("\n\n<q>")
		builder.WriteString(xmlEscapeText(request))
		builder.WriteString("</q>")
	}
	return builder.String()
}

var xmlTextReplacer = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

func xmlEscapeAttr(value string) string {
	var builder strings.Builder
	if err := xml.EscapeText(&builder, []byte(value)); err != nil {
		return ""
	}
	return builder.String()
}

func xmlEscapeText(value string) string {
	return xmlTextReplacer.Replace(value)
}
