package conversation

import (
	"context"
	"sync"
	"time"

	appaudit "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/audit"
	appbilling "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/billing"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	appcompact "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/compact"
	appcm "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/contentmoderation"
	appembedding "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/embedding"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/extraction"
	appstorage "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/objectstorage"
	appprocessing "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/processing"
	apprag "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/rag"
	appupload "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/upload"
	domainbilling "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/billing"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	domainknowledgebase "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/knowledgebase"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"go.uber.org/zap"
)

type routeResolver interface {
	ResolveRoute(ctx context.Context, input channel.ResolveRouteInput) (*channel.ResolvedRoute, error)
	MarkRouteFailure(ctx context.Context, route *channel.ResolvedRoute, cause error)
	MarkRouteSuccess(ctx context.Context, route *channel.ResolvedRoute)
}

// defaultRouteResolver 表示按任务类型解析默认路由的可选能力。
// conversation 只依赖这个窄接口，不直接感知 channel.Service 的具体实现。
type defaultRouteResolver interface {
	ResolveDefaultRoute(ctx context.Context, input channel.ResolveRouteInput) (*channel.ResolvedRoute, error)
}

type knowledgeBaseResolver interface {
	ResolveFiles(ctx context.Context, userID uint, publicIDs []string) ([]domainknowledgebase.KnowledgeBase, []model.FileObject, error)
}

type auditWriter interface {
	Write(ctx context.Context, input appaudit.WriteInput)
}

type basicServiceBillingContextKey struct{}

type basicServiceBillingContext struct {
	UserID         uint
	ConversationID uint
}

// llmGateway 定义会话推理、媒体生成与后台响应管理所需的上游调用能力。
type llmGateway interface {
	Generate(ctx context.Context, route llm.RouteConfig, input llm.GenerateInput) (*llm.GenerateOutput, error)
	GenerateStream(ctx context.Context, route llm.RouteConfig, input llm.GenerateInput, onEvent func(llm.GenerateStreamEvent) error) (*llm.GenerateOutput, error)
	RetrieveOpenAIResponse(ctx context.Context, route llm.RouteConfig, responseID string) (*llm.GenerateOutput, error)
	CancelOpenAIResponse(ctx context.Context, route llm.RouteConfig, responseID string) (*llm.GenerateOutput, error)
}

// Service 封装会话业务能力。
type Service struct {
	cfg                   *config.Runtime
	repo                  repository.ConversationRepository
	cache                 repository.UserSettingCacheRepository
	routeResolver         routeResolver
	llmClient             llmGateway
	uploadSvc             *appupload.Service
	compactSvc            *appcompact.Service
	embeddingSvc          *appembedding.Service
	processingSvc         *appprocessing.Service
	extractSvc            *extraction.Service
	ragSvc                *apprag.Service
	knowledgeBaseResolver knowledgeBaseResolver
	billingSvc            *appbilling.Service
	auditWriter           auditWriter
	storeProvider         appstorage.Provider
	logger                *zap.Logger
	moderationSvc         *appcm.Service
	generationStreams     *generationStreamRegistry
	snapshotCache         sync.Map // conversationID (uint) → *cachedSnapshot
	imageContextCache     *preparedConversationImageCache
}

func (s *Service) llmAttribution() (string, string) {
	if s == nil || s.cfg == nil {
		return "", ""
	}
	cfg := s.cfg.Snapshot()
	return cfg.PublicWebBaseURL, cfg.AppName
}

// AttachmentInput 是消息附件入参（应用层内部传递，无序列化标签）。
type AttachmentInput struct {
	FileObjID              uint
	FileID                 string
	Kind                   string
	FileName               string
	MimeType               string
	DetectedMIME           string
	FileCategory           string
	FileSize               int64
	SHA256                 string
	StoragePath            string
	MetaJSON               string
	PageCount              int
	ProcessingStatus       string
	ProcessingReady        bool
	ProcessingErrorCode    string
	ProcessingErrorMessage string
	ExtractStatus          string
	EmbedStatus            string
	ExtractedText          string
	RAGOptOut              bool // 用户是否关闭该文件的 RAG；RAG 段直接复用，无需重查 DB
	ChunkCount             int  // 向量分块数；RAG 缓存 key 需要
	FileUpdatedAt          time.Time
	Current                bool // 是否为本轮用户显式上传的附件
	MessageRole            string
	ContextMode            string
	DurationSeconds        int64 // 仅生成视频附件使用。
}

// SendMessageInput 定义消息发送请求。
type SendMessageInput struct {
	UserID                  uint
	ConversationID          uint
	RequestID               string
	ContentType             string
	Content                 string
	PlatformModelName       string
	Options                 map[string]any
	ClientRunID             string
	FileIDs                 []string
	KnowledgeBaseIDs        []string
	HTMLVisualPromptEnabled bool
	ParentMessagePublicID   string
	SourceMessagePublicID   string
	BranchReason            string
	Cancelable              bool
	// UsageAuthorization 是请求级计费授权；提示词形状确定后据此把预算预留抬高到预估成本。
	UsageAuthorization *domainbilling.UsageAuthorization
	// OnEvent 用于向调用方推送中间事件（如 rag_search），流式场景使用。
	OnEvent func(eventType string, payload map[string]any) error
}

// SetKnowledgeBaseResolver 注入会话知识库解析器。
func (s *Service) SetKnowledgeBaseResolver(resolver knowledgeBaseResolver) {
	s.knowledgeBaseResolver = resolver
}

// SendMessageResult 返回用户消息与 AI 消息。
type SendMessageResult struct {
	UserMessage         model.Message
	AssistantMessage    model.Message
	MetadataRefreshHint string
	Billable            bool
	UpstreamID          uint
	UpstreamName        string
	PlatformModelName   string
	RoutedBindingCode   string
	UpstreamModelName   string
	UpstreamProtocol    string
	EffectiveOptions    map[string]any
	UsageSpeed          string
	UsageServiceTier    string
	UsageSource         string
	RawUsageJSON        string
	CacheWrite5mTokens  int64
	CacheWrite1hTokens  int64
	// LLMCallCount 是本次运行成功返回的上游 LLM 调用数；按次计费按该计数结算，
	// 中断运行由计费层保底至少计 1 次。
	LLMCallCount    int
	LatencyMS       int64
	DurationSeconds int64
	StartedAt       time.Time
	// 执行了软审核屏障时设置 Moderation；Blocked 表示已撤回。
	Moderation            *MessageModerationOutcome
	postBillingCompaction *postBillingCompactionTask
}

// MessageFeedbackResult 返回反馈后的当前状态（内部传输，不携带序列化标记）。
type MessageFeedbackResult struct {
	MessageID       uint
	MessagePublicID string
	MyFeedback      string
	ThumbsUpCount   int64
	ThumbsDownCount int64
}

// Dependencies 描述会话服务运行所需的应用依赖。
type Dependencies struct {
	Config            *config.Runtime
	Repository        repository.ConversationRepository
	Cache             repository.ConversationCacheRepository
	RouteResolver     routeResolver
	LLMClient         llmGateway
	CompactService    *appcompact.Service
	EmbeddingService  *appembedding.Service
	ProcessingService *appprocessing.Service
	UploadService     *appupload.Service
	ExtractService    *extraction.Service
	RAGService        *apprag.Service
	Logger            *zap.Logger
}

// NewServiceWithRuntime 创建使用运行时配置容器的服务。
// 压缩、embedding、处理流水线、上传、抽取与 RAG 服务全部由组合根装配后注入。
func NewServiceWithRuntime(deps Dependencies) *Service {
	svc := &Service{
		cfg:               deps.Config,
		repo:              deps.Repository,
		cache:             deps.Cache,
		routeResolver:     deps.RouteResolver,
		llmClient:         deps.LLMClient,
		compactSvc:        deps.CompactService,
		embeddingSvc:      deps.EmbeddingService,
		processingSvc:     deps.ProcessingService,
		uploadSvc:         deps.UploadService,
		extractSvc:        deps.ExtractService,
		ragSvc:            deps.RAGService,
		logger:            deps.Logger,
		generationStreams: newGenerationStreamRegistry(deps.Cache, defaultGenerationStreamOptions()),
		imageContextCache: defaultPreparedConversationImageCache(),
	}
	// 注入 LLM 语义压缩回调（在 svc 完全初始化后绑定）
	svc.compactSvc.SetLLMSummarizer(svc.callCompactLLM)
	return svc
}

// Close 停止接收新流式生成，取消活动生成，并等待请求收尾与常驻事件读取器退出。
func (s *Service) Close() {
	if s == nil || s.generationStreams == nil {
		return
	}
	s.generationStreams.close()
}

// UploadErrorSet 让上传服务返回会话域的文件错误哨兵，HTTP 层因此沿用同一套文件错误映射。
func UploadErrorSet() appupload.ErrorSet {
	return appupload.ErrorSet{
		InvalidFileReference: ErrInvalidFileReference,
		InvalidFileName:      ErrInvalidFileName,
		FileNotFound:         ErrFileNotFound,
		FileInUse:            ErrFileInUse,
		StorageQuotaExceeded: ErrStorageQuotaExceeded,
		FileTooLarge:         ErrFileTooLarge,
		MIMEBlocked:          ErrMIMEBlocked,
		DangerousMIMEType:    ErrDangerousMIMEType,
	}
}

// SetBillingService 注入计费服务，用于记录标题、标签、上下文压缩等基础 LLM 服务用量。
func (s *Service) SetBillingService(billingSvc *appbilling.Service) {
	s.billingSvc = billingSvc
}

// SetAuditWriter 注入会话域审计写入器。
func (s *Service) SetAuditWriter(writer auditWriter) {
	s.auditWriter = writer
}

// SetObjectStoreProvider 注入本服务读写媒体产物所用的对象存储；上传与抽取服务的存储由组合根各自注入。
func (s *Service) SetObjectStoreProvider(provider appstorage.Provider) {
	if provider != nil {
		s.storeProvider = provider
	}
}
