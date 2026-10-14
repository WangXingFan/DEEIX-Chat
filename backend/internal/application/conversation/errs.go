package conversation

import (
	"errors"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/apperr"
)

// 错误码与对外文案是前端本地化依赖的 API 契约（apps/web/i18n/messages/*/errors.json 按错误码取文案），
// 因此用 apperr 随哨兵一起声明，传输层用 response.ErrorFrom / response.Describe 直接读取。
// apperr.NewMasked 的第三个参数保留原有内部文本，供日志与错误链使用。
// 仓储层哨兵暂时保留原始身份；transport 必须在边界显式映射为稳定的 apperr 契约。
var (
	// ErrConversationNotFound 会话不存在或无权限。
	ErrConversationNotFound = apperr.New("conversation.not_found", "conversation not found")
	// ErrConversationEventNotFound 对话事件日志不存在。
	ErrConversationEventNotFound = apperr.New("conversation_event.not_found", "conversation event not found")
	// ErrInvalidConversationTitle 会话标题不合法。
	ErrInvalidConversationTitle = apperr.New("conversation.invalid_title", "invalid conversation title")
	// ErrInvalidConversationLabels 会话标签不合法。
	ErrInvalidConversationLabels = apperr.New("request.invalid_conversation_labels", "invalid conversation labels")
	// ErrInvalidFileReference 文件引用无效。
	ErrInvalidFileReference = apperr.New("file.invalid_reference", "invalid file reference")
	// ErrInvalidFileName 文件名不合法。
	ErrInvalidFileName = apperr.New("file.invalid_name", "invalid file name")
	// ErrFileNotFound 文件不存在。
	ErrFileNotFound = apperr.New("file.not_found", "file not found")
	// ErrFileInUse 文件正在被头像、知识库等资源使用。
	ErrFileInUse = apperr.NewMasked("file.in_use", "file is in use", "file in use")
	// ErrStorageQuotaExceeded 文件配额超限。
	// 仓储层仍返回 repository.ErrStorageQuotaExceeded，由应用边界转换为此带契约的哨兵。
	ErrStorageQuotaExceeded = apperr.NewMasked(MessageErrorCodeQuotaExceeded, "quota exceeded", "storage quota exceeded")
	// ErrFileTooLarge 文件过大。
	ErrFileTooLarge = apperr.New("file.too_large", "file too large")
	// ErrMIMEBlocked 文件类型不被允许。
	ErrMIMEBlocked = apperr.NewMasked("file.type_blocked", "file type is not allowed", "mime blocked")
	// ErrDangerousMIMEType 危险文件类型不被允许。
	ErrDangerousMIMEType = apperr.NewMasked("file.type_blocked", "file type is not allowed", "dangerous file type not allowed")
	// ErrFileProcessingNotReady 文件处理尚未就绪。
	ErrFileProcessingNotReady = apperr.NewMasked("file.not_ready", "file processing is not ready", "file processing not ready")
	// ErrFileTooLargeForFullContext 文件过大，无法全文注入。
	ErrFileTooLargeForFullContext = apperr.NewMasked("file.too_large_for_context", "file is too large for full context", "file too large for full context")
	// ErrEmbeddingUnavailable 当前未配置可用 embedding，无法处理大文档 / RAG。
	ErrEmbeddingUnavailable = apperr.NewMasked("file.embedding_unavailable", "embedding is unavailable for current file capability", "embedding unavailable")
	// ErrInvalidKnowledgeBaseReference 知识库不存在、已停用或当前用户不可见。
	ErrInvalidKnowledgeBaseReference = apperr.New(MessageErrorCodeKnowledgeBaseInvalidReference, "invalid knowledge base reference")
	// ErrKnowledgeBaseUnavailable 当前未启用可用的知识库检索能力。
	ErrKnowledgeBaseUnavailable = apperr.NewMasked(MessageErrorCodeKnowledgeBaseUnavailable, "knowledge base retrieval is unavailable", "knowledge base retrieval unavailable")
	// ErrKnowledgeBaseNotReady 所选知识库尚无可检索文件。
	ErrKnowledgeBaseNotReady = apperr.NewMasked(MessageErrorCodeKnowledgeBaseNotReady, "selected knowledge base has no ready files", "knowledge base not ready")
	// ErrKnowledgeBaseScopeEmpty 知识库已启用，但既未显式选择知识库，也未配置默认检索范围。
	ErrKnowledgeBaseScopeEmpty = apperr.New(MessageErrorCodeKnowledgeBaseScopeEmpty, "no knowledge base scope is configured")
	// ErrTooManyMessageFiles 单条消息文件数超限。
	ErrTooManyMessageFiles = apperr.NewMasked("message.too_many_files", "too many files in one message", "too many message files")
	// ErrTooManySelectedSkills 单条消息选择的 Skill 数超限。
	ErrTooManySelectedSkills = apperr.New("message.too_many_selected_skills", "too many selected skills")
	// ErrSkillNotFound 技能不存在或当前用户不可用。
	ErrSkillNotFound = apperr.New("skill.not_found", "skill not found")
	// ErrInvalidSkillUse 技能使用入参不合法。
	ErrInvalidSkillUse = apperr.New("request.invalid_skill_use", "invalid skill use")
	// ErrInvalidMessageBranch 消息分支参数无效。
	ErrInvalidMessageBranch = apperr.New("message.invalid_branch", "invalid message branch")
	// ErrInvalidMessageContent 消息内容不合法。
	ErrInvalidMessageContent = apperr.New("message.invalid_content", "invalid message content")
	// ErrMessageNotFound 消息不存在或无权限。
	ErrMessageNotFound = apperr.New("message.not_found", "message not found")
	// ErrContextArtifactNotFound 上下文证据不存在或无权限。
	ErrContextArtifactNotFound = apperr.New("context_artifact.not_found", "context artifact not found")
	// ErrInvalidMessageFeedback 消息反馈值不合法。
	ErrInvalidMessageFeedback = apperr.New("message.invalid_feedback", "invalid message feedback")
	// ErrMessageFeedbackTargetInvalid 反馈目标消息不合法。
	ErrMessageFeedbackTargetInvalid = apperr.NewMasked("message.feedback_target_invalid", "message feedback target invalid", "invalid message feedback target")
	// ErrMessageEditTargetInvalid 编辑目标消息不合法。
	ErrMessageEditTargetInvalid = apperr.NewMasked("message.edit_target_invalid", "message edit target invalid", "invalid message edit target")
	// ErrMessageEditStateInvalid 当前消息状态不允许编辑。
	ErrMessageEditStateInvalid = apperr.NewMasked("message.edit_state_invalid", "message edit state invalid", "invalid message edit state")
	// ErrMessageForkStateInvalid 当前消息状态不允许 fork。
	ErrMessageForkStateInvalid = apperr.New("conversation.message_fork_state_invalid", "message is still generating")
	// ErrMessageForkTargetInvalid 当前消息角色不允许 fork。
	ErrMessageForkTargetInvalid = apperr.New("conversation.message_fork_target_invalid", "only assistant messages can be forked")
	// ErrMessageForkHistoryIncomplete 消息祖先链超过安全上限或已损坏，无法完整 fork。
	ErrMessageForkHistoryIncomplete = apperr.New("conversation.message_fork_history_incomplete", "message history is too deep or incomplete")
	// ErrMessageDeleteTargetInvalid 当前消息角色不允许删除。
	ErrMessageDeleteTargetInvalid = apperr.New("conversation.message_delete_target_invalid", "only user or assistant messages can be deleted")
	// ErrMessageDeleteStateInvalid 当前消息状态不允许删除。
	ErrMessageDeleteStateInvalid = apperr.New("conversation.message_delete_state_invalid", "message is still generating")
	// ErrMessageDeleteRootInvalid 会话第一条消息不允许删除，否则历史将以助手消息开头。
	ErrMessageDeleteRootInvalid = apperr.New("conversation.message_delete_root_invalid", "cannot delete the first message")
	// ErrMessageParentDeleted 父消息已被删除，无法再挂在其下创建新消息。
	ErrMessageParentDeleted    = apperr.New("conversation.message_parent_deleted", "parent message was deleted")
	ErrModelRouteNotConfigured = apperr.NewMasked("llm.model_route_not_configured", "model route is not configured", "model route not configured")
	// ErrModelAccessDenied 当前用户无权使用此模型。
	ErrModelAccessDenied = apperr.NewMasked("llm.model_access_denied", "you do not have access to this model", "model access denied by group policy")
	// ErrUpstreamRequestFailed 上游请求失败。对外错误码与文案取决于上游错误特征，由 transport 按 MessageErrorCode /
	// messageErrorSummary 逐次判定，因此不在这里声明。
	ErrUpstreamRequestFailed = errors.New("upstream request failed")
	// ErrUpstreamEmptyResponse 上游返回空响应。
	ErrUpstreamEmptyResponse = apperr.NewMasked(MessageErrorCodeUpstreamEmptyResponse, "model returned empty response", "upstream returned empty response")
	// ErrMessageGenerationCanceled 用户主动停止生成。
	ErrMessageGenerationCanceled = apperr.New("conversation_run.canceled", "message generation canceled")
	// ErrMessageGenerationInterrupted 活跃生成租约消失，当前生成流无法继续恢复。
	ErrMessageGenerationInterrupted = apperr.New("conversation_run.stream_interrupted", "generation stream was interrupted; retry this message")
	// ErrDuplicateMessageGenerationRun 表示客户端重复提交同一个生成 run。
	ErrDuplicateMessageGenerationRun = apperr.NewMasked("message_generation_run.already_exists", "message generation run already exists", "duplicate message generation run")
)

// mapMessageWriteError 把仓储层在消息写入与删除路径上返回的哨兵转换为应用层契约哨兵。
// 仓储层只表达存储语义，对外错误码与文案必须由应用边界收敛（约定见本文件顶部），
// 否则并发删除父消息、生成中删除等分支会退化成无错误码的 500。
func mapMessageWriteError(err error) error {
	switch {
	case errors.Is(err, repository.ErrMessageParentDeleted):
		return ErrMessageParentDeleted
	case errors.Is(err, repository.ErrMessageDeleteStateInvalid):
		return ErrMessageDeleteStateInvalid
	case errors.Is(err, repository.ErrMessageDeleteRootInvalid):
		return ErrMessageDeleteRootInvalid
	default:
		return err
	}
}
