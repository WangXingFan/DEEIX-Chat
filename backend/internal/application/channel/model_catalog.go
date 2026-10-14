package channel

import (
	"encoding/json"
	"strings"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

const (
	// TaskTypeChat 表示普通聊天或内部文本任务。
	TaskTypeChat = "chat"

	modelKindChat  = "chat"
	modelKindAudio = "audio"

	compatibleOpenAI     = "openai"
	compatibleAnthropic  = "anthropic"
	compatibleGoogle     = "google"
	compatibleXAI        = "xai"
	compatibleOpenRouter = "openrouter"
	compatibleCustom     = "custom"

	protocolGeminiInteractions = llm.AdapterGeminiInteractions
)

var protocolDefaultKindOrder = []string{
	modelKindChat,
	modelKindAudio,
}

func normalizeCompatible(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case compatibleAnthropic:
		return compatibleAnthropic
	case compatibleGoogle:
		return compatibleGoogle
	case compatibleXAI:
		return compatibleXAI
	case compatibleOpenRouter:
		return compatibleOpenRouter
	case compatibleCustom:
		return compatibleCustom
	case "", compatibleOpenAI:
		return compatibleOpenAI
	default:
		return ""
	}
}

func protocolDefaultsForCompatible(compatible string) string {
	defaults := map[string]string{}
	for kind, protocol := range systemFallbackProtocols(normalizeCompatible(compatible)) {
		defaults[kind] = protocol
	}
	payload, _ := json.Marshal(defaults)
	return string(payload)
}

func normalizeProtocolDefaultsJSON(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return `{}`, nil
	}
	var payload map[string]*string
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload == nil {
		return "", ErrInvalidJSONConfig
	}

	defaults := make(map[string]string)
	for _, kind := range protocolDefaultKindOrder {
		value, ok := payload[kind]
		if !ok || value == nil {
			continue
		}
		protocol := strings.TrimSpace(strings.ToLower(*value))
		if protocol == "" {
			continue
		}
		if !isKnownProtocol(protocol) {
			return "", ErrInvalidAdapter
		}
		if !isProtocolAllowedForKind(kind, protocol) {
			return "", ErrInvalidAdapter
		}
		defaults[kind] = protocol
	}

	normalized, _ := json.Marshal(defaults)
	return string(normalized), nil
}

func systemFallbackProtocols(compatible string) map[string]string {
	switch normalizeCompatible(compatible) {
	case compatibleOpenAI:
		return map[string]string{
			modelKindChat:  llm.AdapterOpenAIChatCompletions,
			modelKindAudio: llm.AdapterOpenAIChatCompletions,
		}
	case compatibleAnthropic:
		return map[string]string{
			modelKindChat:  llm.AdapterAnthropicMessages,
			modelKindAudio: llm.AdapterAnthropicMessages,
		}
	case compatibleGoogle:
		return map[string]string{
			modelKindChat:  llm.AdapterGoogleGenerateContent,
			modelKindAudio: llm.AdapterGoogleGenerateContent,
		}
	case compatibleXAI:
		return map[string]string{
			modelKindChat:  llm.AdapterXAIResponses,
			modelKindAudio: llm.AdapterXAIResponses,
		}
	case compatibleOpenRouter:
		return map[string]string{
			modelKindChat:  llm.AdapterOpenRouterResponses,
			modelKindAudio: llm.AdapterOpenRouterResponses,
		}
	case compatibleCustom:
		return map[string]string{
			modelKindChat:  llm.AdapterOpenAIChatCompletions,
			modelKindAudio: llm.AdapterOpenAIChatCompletions,
		}
	default:
		return map[string]string{}
	}
}

func isKnownProtocol(raw string) bool {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case llm.AdapterOpenAIResponses,
		llm.AdapterOpenRouterChat,
		llm.AdapterOpenRouterResponses,
		llm.AdapterOpenAIChatCompletions,
		llm.AdapterAnthropicMessages,
		llm.AdapterGoogleGenerateContent,
		llm.AdapterXAIResponses,
		protocolGeminiInteractions:
		return true
	default:
		return false
	}
}

func resolveRouteProtocol(explicit string, upCompatible string, defaultsJSON string, kindsJSON string) (string, error) {
	kind := primaryKindFromKinds(kindsJSON)
	if protocol := strings.TrimSpace(strings.ToLower(explicit)); protocol != "" {
		if !isKnownProtocol(protocol) {
			return "", ErrInvalidAdapter
		}
		if !isProtocolAllowedForKinds(kindsJSON, protocol) {
			return "", ErrInvalidAdapter
		}
		return protocol, nil
	}

	if kind == "" {
		return "", ErrProtocolRequired
	}
	if protocol := protocolDefaultForKind(defaultsJSON, kind); protocol != "" {
		return protocol, nil
	}
	if protocol := systemFallbackProtocols(upCompatible)[kind]; protocol != "" {
		return protocol, nil
	}
	return "", ErrProtocolRequired
}

// resolveRouteProtocols 解析批量导入时的协议列表：显式值与系统默认值都收敛为单一协议绑定。
func resolveRouteProtocols(explicit []string, upCompatible string, defaultsJSON string, kindsJSON string) ([]string, error) {
	protocols := make([]string, 0, len(explicit))
	seen := make(map[string]struct{}, len(explicit))
	for _, raw := range explicit {
		value := strings.TrimSpace(strings.ToLower(raw))
		if value == "" {
			continue
		}
		protocol, err := resolveRouteProtocol(value, upCompatible, defaultsJSON, kindsJSON)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[protocol]; ok {
			continue
		}
		seen[protocol] = struct{}{}
		protocols = append(protocols, protocol)
	}
	if len(protocols) > 0 {
		if len(protocols) > 1 {
			return nil, ErrInvalidRouteProtocolCombination
		}
		return protocols, nil
	}

	protocol, err := resolveRouteProtocol("", upCompatible, defaultsJSON, kindsJSON)
	if err != nil {
		return nil, err
	}
	return []string{protocol}, nil
}

func isProtocolAllowedForKinds(kindsJSON string, protocol string) bool {
	kinds := parseKinds(kindsJSON)
	if len(kinds) == 0 {
		return true
	}
	for _, kind := range kinds {
		if isProtocolAllowedForKind(kind, protocol) {
			return true
		}
	}
	return false
}

func protocolDefaultForKind(defaultsJSON string, kind string) string {
	defaults := make(map[string]*string)
	if err := json.Unmarshal([]byte(strings.TrimSpace(defaultsJSON)), &defaults); err != nil {
		return ""
	}
	value, ok := defaults[kind]
	if !ok || value == nil {
		return ""
	}
	protocol := strings.TrimSpace(strings.ToLower(*value))
	if !isKnownProtocol(protocol) {
		return ""
	}
	if !isProtocolAllowedForKind(kind, protocol) {
		return ""
	}
	return protocol
}

func isProtocolAllowedForKind(kind string, protocol string) bool {
	switch kind {
	case modelKindChat:
		switch protocol {
		case llm.AdapterOpenAIResponses,
			llm.AdapterOpenRouterChat,
			llm.AdapterOpenRouterResponses,
			llm.AdapterOpenAIChatCompletions,
			llm.AdapterAnthropicMessages,
			llm.AdapterGoogleGenerateContent,
			protocolGeminiInteractions,
			llm.AdapterXAIResponses:
			return true
		default:
			return false
		}
	case modelKindAudio:
		switch protocol {
		case llm.AdapterOpenAIResponses,
			llm.AdapterOpenRouterChat,
			llm.AdapterOpenRouterResponses,
			llm.AdapterOpenAIChatCompletions,
			llm.AdapterAnthropicMessages,
			llm.AdapterGoogleGenerateContent,
			llm.AdapterXAIResponses:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// NormalizeTaskType 归一化模型路由任务类型。
// 未传任务类型时按聊天处理，保留旧调用方的默认行为。
func NormalizeTaskType(raw string) string {
	_ = raw
	return TaskTypeChat
}

// IsRouteAllowedForTask 判断指定模型 kind 与协议是否可服务聊天任务。
func IsRouteAllowedForTask(taskType string, kindsJSON string, protocol string) bool {
	_ = taskType
	kinds := parseKinds(kindsJSON)
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	if len(kinds) == 0 {
		return isProtocolAllowedForKind(modelKindChat, protocol) || isProtocolAllowedForKind(modelKindAudio, protocol)
	}
	for _, kind := range kinds {
		if (kind == modelKindChat || kind == modelKindAudio) && isProtocolAllowedForKind(kind, protocol) {
			return true
		}
	}
	return false
}

// hasModelKind 判断模型 kind 列表是否包含目标能力。
func hasModelKind(kinds []string, target string) bool {
	for _, kind := range kinds {
		if kind == target {
			return true
		}
	}
	return false
}

func primaryKindFromKinds(kindsJSON string) string {
	kinds := parseKinds(kindsJSON)
	for _, candidate := range protocolDefaultKindOrder {
		for _, kind := range kinds {
			if kind == candidate {
				return kind
			}
		}
	}
	return ""
}

func inferKindsJSON(platformModelName string) string {
	code := strings.ToLower(strings.TrimSpace(platformModelName))
	switch {
	case strings.HasPrefix(code, "gpt-4o-audio"):
		return `["audio"]`
	default:
		return `["chat"]`
	}
}
