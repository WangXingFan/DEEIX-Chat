package conversation

import "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/apperr"

// 由 transport 层直接判定的请求错误（路径参数、必填校验、运行时依赖缺失等）。
// 错误码与文案是前端依赖的 API 契约，改动需同步 apps/web/i18n/messages/*/errors.json。
var (
	errAtLeastOneOfFileNameOrRAGOptOutRequired = apperr.New("request.required", "at least one of file_name or rag_opt_out is required")
	errConversationNoTitleableContent          = apperr.New("conversation.no_titleable_content", "conversation has no titleable content")
	errFileExtractNotReady                     = apperr.New("file.extract_not_ready", "file extract is not ready")
	errFileEmbeddingUnavailable                = apperr.New("file.embedding_unavailable", "embedding is unavailable for this file size")
	errFileRequired                            = apperr.New("file.required", "file is required")
	errGenerationStreamNotFound                = apperr.New("conversation_run.stream_not_found", "generation stream not found")
	errInvalidBeforeMessageID                  = apperr.New("before_message.invalid_id", "invalid before message id")
	errInvalidContextArtifactID                = apperr.New("context_artifact.invalid_id", "invalid context artifact id")
	errInvalidConversationID                   = apperr.New("conversation.invalid_id", "invalid conversation id")
	errInvalidFileID                           = apperr.New("file.invalid_id", "invalid file id")
	errInvalidFile                             = apperr.New("request.invalid_file", "invalid file")
	errInvalidFileStream                       = apperr.New("file.invalid_stream", "invalid file stream")
	errInvalidMessageID                        = apperr.New("message.invalid_id", "invalid message id")
	errInvalidRunID                            = apperr.New("run.invalid_id", "invalid run id")
	errInvalidRunIDs                           = apperr.New("request.invalid_run_ids", "invalid run ids")
	errLabelsRequired                          = apperr.New("request.required", "labels are required")
	errStorageQuotaExceeded                    = apperr.New("quota.exceeded", "storage quota exceeded")
)
