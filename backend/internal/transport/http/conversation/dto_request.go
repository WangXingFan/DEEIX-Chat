package conversation

// CreateConversationRequest 创建会话请求。
type CreateConversationRequest struct {
	Title string `json:"title,omitempty" binding:"max=255"`
	Model string `json:"model,omitempty" binding:"max=128"`
}

// RenameConversationRequest 重命名会话请求。
type RenameConversationRequest struct {
	Title string `json:"title" binding:"required,max=255"`
}

// UpdateConversationLabelsRequest 更新会话标签请求。
type UpdateConversationLabelsRequest struct {
	Labels *[]string `json:"labels" binding:"required,max=6,dive,max=24" maxLength:"24"`
}

// SetConversationStarRequest 设置星标请求。
type SetConversationStarRequest struct {
	Starred *bool `json:"starred" binding:"required"`
}

// SetConversationArchiveRequest 设置归档状态请求。
type SetConversationArchiveRequest struct {
	Archived *bool `json:"archived" binding:"required"`
}

// RenameFileRequest 文件重命名请求。
type RenameFileRequest struct {
	FileName string `json:"fileName" binding:"required,max=255"`
}

// UpdateFileRequest 文件更新请求，file_name 和 rag_opt_out 至少填一个。
type UpdateFileRequest struct {
	FileName  *string `json:"fileName,omitempty"`
	RAGOptOut *bool   `json:"ragOptOut,omitempty"`
}

// GetFileProcessingStatusesRequest 批量文件处理状态查询请求。
type GetFileProcessingStatusesRequest struct {
	FileIDs []string `json:"fileIDs" binding:"required,min=1,max=100,dive,required,max=64"`
}

// SubmitFileEmbeddingsRequest 批量提交文件向量化请求。
type SubmitFileEmbeddingsRequest struct {
	FileIDs []string `json:"fileIDs" binding:"required,min=1,max=100,dive,required,max=64"`
}

// GetConversationRunStatusesRequest 批量运行状态查询请求。
type GetConversationRunStatusesRequest struct {
	RunIDs []string `json:"runIDs" binding:"required,min=1,max=100,dive,required,max=64"`
}

// SendMessageRequest 发送消息请求。
type SendMessageRequest struct {
	ContentType             string         `json:"contentType" binding:"required,oneof=text markdown image file mixed"`
	Content                 string         `json:"content" binding:"required"`
	Model                   string         `json:"model,omitempty" binding:"omitempty,max=128"`
	Options                 map[string]any `json:"options,omitempty"`
	ClientRunID             string         `json:"clientRunID,omitempty" binding:"omitempty,max=64"`
	FileIDs                 []string       `json:"fileIDs,omitempty" binding:"max=20"`
	SkillIDs                []uint         `json:"skillIDs,omitempty" binding:"max=128"`
	KnowledgeBaseIDs        []string       `json:"knowledgeBaseIDs,omitempty" binding:"max=8,dive,required,max=32"`
	HTMLVisualPromptEnabled bool           `json:"htmlVisualPrompt,omitempty"`
	ParentMessagePublicID   string         `json:"parentMessagePublicID,omitempty" binding:"omitempty,max=32"`
	SourceMessagePublicID   string         `json:"sourceMessagePublicID,omitempty" binding:"omitempty,max=32"`
	BranchReason            string         `json:"branchReason,omitempty" binding:"omitempty,oneof=default retry edit"`
}

// SetMessageFeedbackRequest 设置消息反馈请求。
type SetMessageFeedbackRequest struct {
	Feedback string `json:"feedback,omitempty" binding:"omitempty,oneof=up down"`
}

// UpdateMessageRequest 更新消息内容请求。
type UpdateMessageRequest struct {
	Content string `json:"content" binding:"required"`
}
