package conversation

import (
	"encoding/json"
	"strings"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

const (
	modelOptionPolicyAllowlist = "allowlist"
	modelOptionPolicyDenylist  = "denylist"
	modelOptionPolicyDisabled  = "disabled"
)

var hardDeniedModelOptionPaths = [][]string{
	{"model"},
	{"messages"},
	{"input"},
	{"instructions"},
	{"prompt"},
	{"system"},
	{"systemInstruction"},
	{"headers"},
	{"api_key"},
	{"apiKey"},
	{"base_url"},
	{"baseURL"},
	{"stream"},
	{"previous_response_id"},
	{"prompt_cache_key"},
	{"prompt_cache_options"},
	{"prompt_cache_breakpoint"},
	{"prompt_cache_retention"},
}

type modelOptionPolicyConfig struct {
	Mode                  string
	AllowedPathsJSON      string
	DeniedPathsJSON       string
	ModelCapabilitiesJSON string
}

func filterModelOptions(options map[string]any, protocol string, cfg modelOptionPolicyConfig) map[string]any {
	mode := strings.TrimSpace(cfg.Mode)
	if mode == "" {
		mode = modelOptionPolicyAllowlist
	}
	if mode == modelOptionPolicyDisabled {
		return nil
	}

	protocolKey := modelOptionPolicyProtocolKey(protocol)
	defaultOptions := modelCapabilityDefaultOptions(cfg.ModelCapabilitiesJSON)
	policyOptions := mergeModelOptionDefaults(
		defaultOptions,
		options,
		modelCapabilityLockedOptionPaths(cfg.ModelCapabilitiesJSON),
	)
	if len(policyOptions) == 0 {
		return nil
	}
	delete(policyOptions, "tools")
	denied := append([][]string{}, hardDeniedModelOptionPaths...)

	var filtered map[string]any
	switch mode {
	case modelOptionPolicyDenylist:
		denied = append(denied, modelOptionPathsForProtocol(cfg.DeniedPathsJSON, protocolKey)...)
		filtered = policyOptions
	default:
		filtered = make(map[string]any)
		for _, path := range modelOptionPathsForProtocol(cfg.AllowedPathsJSON, protocolKey) {
			copyModelOptionPath(filtered, policyOptions, path)
		}
	}

	for _, path := range denied {
		deleteModelOptionPath(filtered, path)
	}
	sanitizeModelOptionValues(filtered, protocolKey)
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// modelCapabilityDefaultOptions 提取管理员在模型能力 JSON 中声明的默认请求参数。
func modelCapabilityDefaultOptions(raw string) map[string]any {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	var config struct {
		DefaultOptions map[string]any `json:"defaultOptions"`
	}
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return nil
	}
	return cloneModelOptionMap(config.DefaultOptions)
}

func modelCapabilityLockedOptionPaths(raw string) [][]string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	var config struct {
		LockedOptionPaths []string `json:"lockedOptionPaths"`
	}
	if err := json.Unmarshal([]byte(value), &config); err != nil {
		return nil
	}
	paths := make([][]string, 0, len(config.LockedOptionPaths))
	for _, value := range config.LockedOptionPaths {
		if path := splitModelOptionPath(value); len(path) > 0 {
			paths = append(paths, path)
		}
	}
	return paths
}

// mergeModelOptionDefaults 以能力默认值为基础合并本次显式参数，并对锁定路径恢复默认值。
func mergeModelOptionDefaults(defaults map[string]any, options map[string]any, lockedPaths [][]string) map[string]any {
	merged := cloneModelOptionMap(defaults)
	if merged == nil {
		merged = make(map[string]any, len(options))
	}
	mergeModelOptionMap(merged, options)
	for _, path := range lockedPaths {
		if value, ok := readModelOptionPath(defaults, path); ok {
			writeModelOptionPath(merged, path, cloneModelOptionValue(value))
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func mergeModelOptionMap(dst map[string]any, src map[string]any) {
	for key, value := range src {
		srcMap, srcIsMap := value.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)
		if srcIsMap && dstIsMap && dstMap != nil {
			mergeModelOptionMap(dstMap, srcMap)
			continue
		}
		dst[key] = cloneModelOptionValue(value)
	}
}

func modelOptionStringValue(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func sanitizeModelOptionValues(options map[string]any, protocolKey string) {
	if len(options) == 0 {
		return
	}
	switch protocolKey {
	case "openai_chat_completions", "openai_responses", "openrouter_responses":
		sanitizeOpenAIServiceTier(options)
	}
}

func sanitizeOpenAIServiceTier(options map[string]any) {
	serviceTier, ok := options["service_tier"]
	if !ok {
		return
	}
	value, ok := serviceTier.(string)
	if !ok {
		delete(options, "service_tier")
		return
	}
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "default", "flex", "priority":
		options["service_tier"] = strings.TrimSpace(strings.ToLower(value))
	default:
		delete(options, "service_tier")
	}
}

func modelOptionPolicyProtocolKey(protocol string) string {
	switch llm.NormalizeAdapter(protocol) {
	case "openai":
		return "openai_responses"
	case "openrouter":
		return "openrouter_responses"
	case "anthropic", "claude":
		return "anthropic_messages"
	case "xai", "grok":
		return "xai_responses"
	case "google", "gemini":
		return "gemini_generate_content"
	case llm.AdapterGoogleGenerateContent:
		return "gemini_generate_content"
	case llm.AdapterGeminiInteractions:
		return "gemini_interactions"
	case llm.AdapterOpenAIChatCompletions:
		return "openai_chat_completions"
	case llm.AdapterOpenRouterChat:
		return "openrouter_chat_completions"
	case llm.AdapterOpenRouterResponses:
		return "openrouter_responses"
	case llm.AdapterAnthropicMessages:
		return "anthropic_messages"
	case llm.AdapterXAIResponses:
		return "xai_responses"
	default:
		return "openai_responses"
	}
}

func modelOptionPathsForProtocol(raw string, protocol string) [][]string {
	var config map[string][]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &config); err != nil {
		return nil
	}
	paths := make([][]string, 0, len(config["default"])+len(config[protocol]))
	for _, value := range append(config["default"], config[protocol]...) {
		if path := splitModelOptionPath(value); len(path) > 0 {
			paths = append(paths, path)
		}
	}
	return paths
}

func splitModelOptionPath(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return nil
	}
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if part == "" {
			return nil
		}
	}
	return parts
}

func copyModelOptionPath(dst map[string]any, src map[string]any, path []string) {
	value, ok := copyModelOptionValueAtPath(src, path)
	if !ok {
		return
	}
	mergeModelOptionPathValue(dst, value)
}

func copyModelOptionValueAtPath(value any, path []string) (any, bool) {
	if len(path) == 0 {
		return cloneModelOptionValue(value), true
	}
	switch typed := value.(type) {
	case map[string]any:
		child, ok := typed[path[0]]
		if !ok {
			return nil, false
		}
		copied, ok := copyModelOptionValueAtPath(child, path[1:])
		if !ok {
			return nil, false
		}
		return map[string]any{path[0]: copied}, true
	case []any:
		items := make([]any, len(typed))
		matched := false
		for index, item := range typed {
			copied, ok := copyModelOptionValueAtPath(item, path)
			if ok {
				items[index] = copied
				matched = true
			} else {
				items[index] = map[string]any{}
			}
		}
		if !matched {
			return nil, false
		}
		return items, true
	default:
		return nil, false
	}
}

func mergeModelOptionPathValue(dst map[string]any, value any) {
	payload, ok := value.(map[string]any)
	if !ok {
		return
	}
	mergeModelOptionPathMap(dst, payload)
}

func mergeModelOptionPathMap(dst map[string]any, src map[string]any) {
	for key, value := range src {
		if existingMap, ok := dst[key].(map[string]any); ok {
			if incomingMap, ok := value.(map[string]any); ok {
				mergeModelOptionPathMap(existingMap, incomingMap)
				continue
			}
		}
		if existingItems, ok := dst[key].([]any); ok {
			if incomingItems, ok := value.([]any); ok {
				dst[key] = mergeModelOptionPathArray(existingItems, incomingItems)
				continue
			}
		}
		dst[key] = cloneModelOptionValue(value)
	}
}

func mergeModelOptionPathArray(existing []any, incoming []any) []any {
	result := make([]any, len(existing))
	for index, item := range existing {
		result[index] = cloneModelOptionValue(item)
	}
	for index, item := range incoming {
		if index >= len(result) {
			result = append(result, cloneModelOptionValue(item))
			continue
		}
		existingMap, existingOK := result[index].(map[string]any)
		incomingMap, incomingOK := item.(map[string]any)
		if existingOK && incomingOK {
			mergeModelOptionPathMap(existingMap, incomingMap)
			continue
		}
		result[index] = cloneModelOptionValue(item)
	}
	return result
}

func readModelOptionPath(src map[string]any, path []string) (any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	current := src
	for index, segment := range path {
		value, ok := current[segment]
		if !ok {
			return nil, false
		}
		if index == len(path)-1 {
			return value, true
		}
		next, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return nil, false
}

func writeModelOptionPath(dst map[string]any, path []string, value any) {
	current := dst
	for index, segment := range path {
		if index == len(path)-1 {
			current[segment] = value
			return
		}
		next, ok := current[segment].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[segment] = next
		}
		current = next
	}
}

func deleteModelOptionPath(dst map[string]any, path []string) {
	if len(path) == 0 || len(dst) == 0 {
		return
	}
	current := dst
	for index, segment := range path {
		if index == len(path)-1 {
			delete(current, segment)
			return
		}
		next, ok := current[segment].(map[string]any)
		if !ok {
			return
		}
		current = next
	}
}

func cloneModelOptionMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = cloneModelOptionValue(value)
	}
	return dst
}

func cloneModelOptionValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneModelOptionMap(typed)
	case []any:
		items := make([]any, len(typed))
		for index, item := range typed {
			items[index] = cloneModelOptionValue(item)
		}
		return items
	default:
		return typed
	}
}

// shouldApplyReasoningPassbackRequestOptions 判断本轮是否需要下发厂商私有的回传配套入参。
//
// 三个条件缺一不可：
//   - 回传实际生效（路由能力 AND 用户设置），否则等于付费读历史推理却没有历史推理可读；
//   - 该路由确有配套入参要求；
//   - 本轮真实发送的历史里已存在非空推理。这些入参只影响「历史」思维链的处理方式，
//     首轮或非思考模型下发它没有收益，且能规避自建后端把未知顶层字段判为非法入参。
func shouldApplyReasoningPassbackRequestOptions(
	passbackEnabled bool,
	required map[string]any,
	messages []llm.Message,
) bool {
	if !passbackEnabled || len(required) == 0 {
		return false
	}
	return promptCarriesAssistantReasoning(messages)
}

// promptCarriesAssistantReasoning 判断本轮真实发送的历史里是否已有非空 assistant 推理内容。
func promptCarriesAssistantReasoning(messages []llm.Message) bool {
	for _, item := range messages {
		if item.Role == "assistant" && strings.TrimSpace(item.ReasoningContent) != "" {
			return true
		}
	}
	return false
}

// withReasoningPassbackRequestOptions 补齐厂商要求的回传配套入参。
//
// 该入参属于协议正确性而非用户偏好，因此绕过管理员选项白名单——白名单收窄时不应让回传
// 静默退化成「传了字段但模型不读」。但用户或管理员显式声明过的值一律不覆盖：除了已过滤
// 结果，还需回看 rawOptions 与模型能力 defaultOptions，因为白名单模式会把未放行的键丢掉，
// 只看 options 会把管理员刻意设的 false 覆盖成 true。
func withReasoningPassbackRequestOptions(
	options map[string]any,
	required map[string]any,
	rawOptions map[string]any,
	capabilitiesJSON string,
) map[string]any {
	if len(required) == 0 {
		return options
	}
	defaults := modelCapabilityDefaultOptions(capabilitiesJSON)
	for key, value := range required {
		if _, ok := options[key]; ok {
			continue
		}
		if _, ok := rawOptions[key]; ok {
			continue
		}
		if _, ok := defaults[key]; ok {
			continue
		}
		if options == nil {
			options = make(map[string]any, len(required))
		}
		options[key] = value
	}
	return options
}
