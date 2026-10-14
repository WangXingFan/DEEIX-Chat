package conversation

import (
	"testing"

	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

func TestModelOptionPolicyProtocolKeyNormalizesProviderAliases(t *testing.T) {
	tests := map[string]string{
		"xai":       "xai_responses",
		"grok":      "xai_responses",
		"anthropic": "anthropic_messages",
		"claude":    "anthropic_messages",
		"google":    "gemini_generate_content",
		"gemini":    "gemini_generate_content",
		"openai":    "openai_responses",
	}

	for protocol, expected := range tests {
		if got := modelOptionPolicyProtocolKey(protocol); got != expected {
			t.Fatalf("expected %s to normalize to %s, got %s", protocol, expected, got)
		}
	}
}

func TestFilterModelOptionsAllowlistUsesDefaultAndProtocolPaths(t *testing.T) {
	filtered := filterModelOptions(map[string]any{
		"temperature":  0.7,
		"service_tier": "PRIORITY",
		"model":        "override",
		"reasoning": map[string]any{
			"effort":  "high",
			"summary": "auto",
			"extra":   true,
		},
		"text": map[string]any{
			"verbosity": "low",
		},
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}, llm.AdapterOpenAIResponses, modelOptionPolicyConfig{
		Mode:             modelOptionPolicyAllowlist,
		AllowedPathsJSON: config.DefaultModelOptionAllowedPathsJSON(),
		DeniedPathsJSON:  `{"default":["reasoning.effort"]}`,
	})

	if filtered["temperature"] != 0.7 {
		t.Fatalf("expected temperature to pass, got %#v", filtered)
	}
	if filtered["service_tier"] != "priority" {
		t.Fatalf("expected service_tier to pass, got %#v", filtered)
	}
	if _, ok := filtered["model"]; ok {
		t.Fatalf("expected model to be denied, got %#v", filtered)
	}
	reasoning := filtered["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" || reasoning["summary"] != "auto" {
		t.Fatalf("expected allowed reasoning fields, got %#v", reasoning)
	}
	if _, ok := reasoning["extra"]; ok {
		t.Fatalf("expected unlisted reasoning.extra to be removed, got %#v", reasoning)
	}
	if _, ok := filtered["stream_options"]; ok {
		t.Fatalf("expected chat-only stream_options to be removed for responses, got %#v", filtered)
	}
}

func TestFilterModelOptionsAllowsGeminiInteractionResponseFormatArray(t *testing.T) {
	filtered := filterModelOptions(map[string]any{
		"response_format": []any{
			map[string]any{
				"type":      "text",
				"mime_type": "application/json",
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"answer": map[string]any{"type": "string"},
					},
				},
			},
		},
	}, llm.AdapterGeminiInteractions, modelOptionPolicyConfig{
		Mode:             modelOptionPolicyAllowlist,
		AllowedPathsJSON: config.DefaultModelOptionAllowedPathsJSON(),
		DeniedPathsJSON:  config.DefaultModelOptionDeniedPathsJSON(),
	})

	formats, ok := filtered["response_format"].([]any)
	if !ok || len(formats) != 1 {
		t.Fatalf("expected Gemini Interactions response_format array to pass, got %#v", filtered)
	}
	textFormat := formats[0].(map[string]any)
	schema, ok := textFormat["schema"].(map[string]any)
	if !ok || schema["type"] != "object" {
		t.Fatalf("expected whitelisted text schema to pass, got %#v", textFormat)
	}
}

func TestFilterModelOptionsAppliesCapabilityDefaultOptions(t *testing.T) {
	filtered := filterModelOptions(map[string]any{
		"reasoning": map[string]any{
			"effort": "high",
		},
	}, llm.AdapterOpenAIResponses, modelOptionPolicyConfig{
		Mode:             modelOptionPolicyAllowlist,
		AllowedPathsJSON: config.DefaultModelOptionAllowedPathsJSON(),
		DeniedPathsJSON:  config.DefaultModelOptionDeniedPathsJSON(),
		ModelCapabilitiesJSON: `{
			"defaultOptions": {
				"reasoning": {"effort": "medium", "summary": "auto"},
				"text": {"verbosity": "low"},
				"model": "blocked"
			}
		}`,
	})

	reasoning := filtered["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" || reasoning["summary"] != "auto" {
		t.Fatalf("expected explicit option to override default and default summary to remain, got %#v", reasoning)
	}
	text := filtered["text"].(map[string]any)
	if text["verbosity"] != "low" {
		t.Fatalf("expected default text verbosity to pass, got %#v", filtered)
	}
	if _, ok := filtered["model"]; ok {
		t.Fatalf("expected hard-denied default option to be removed, got %#v", filtered)
	}
}

func TestFilterModelOptionsAppliesLockedCapabilityDefaultOptions(t *testing.T) {
	filtered := filterModelOptions(map[string]any{
		"reasoning": map[string]any{
			"effort": "high",
		},
		"text": map[string]any{
			"verbosity": "high",
		},
	}, llm.AdapterOpenAIResponses, modelOptionPolicyConfig{
		Mode:             modelOptionPolicyAllowlist,
		AllowedPathsJSON: config.DefaultModelOptionAllowedPathsJSON(),
		DeniedPathsJSON:  config.DefaultModelOptionDeniedPathsJSON(),
		ModelCapabilitiesJSON: `{
			"defaultOptions": {
				"reasoning": {"effort": "low"},
				"text": {"verbosity": "low"},
				"previous_response_id": "resp_blocked"
			},
			"lockedOptionPaths": ["reasoning.effort", "text.verbosity", "previous_response_id"]
		}`,
	})

	reasoning := filtered["reasoning"].(map[string]any)
	if reasoning["effort"] != "low" {
		t.Fatalf("expected locked default reasoning effort to override explicit option, got %#v", reasoning)
	}
	text := filtered["text"].(map[string]any)
	if text["verbosity"] != "low" {
		t.Fatalf("expected locked default text verbosity to override explicit option, got %#v", text)
	}
	if _, ok := filtered["previous_response_id"]; ok {
		t.Fatalf("expected hard-denied locked default option to be removed, got %#v", filtered)
	}
}

func TestFilterModelOptionsRejectsUnsupportedOpenAIServiceTier(t *testing.T) {
	for _, serviceTier := range []string{"auto", "scale", "unknown"} {
		t.Run(serviceTier, func(t *testing.T) {
			filtered := filterModelOptions(map[string]any{
				"temperature":  0.7,
				"service_tier": serviceTier,
			}, llm.AdapterOpenAIResponses, modelOptionPolicyConfig{
				Mode:             modelOptionPolicyAllowlist,
				AllowedPathsJSON: config.DefaultModelOptionAllowedPathsJSON(),
				DeniedPathsJSON:  config.DefaultModelOptionDeniedPathsJSON(),
			})

			if _, ok := filtered["service_tier"]; ok {
				t.Fatalf("expected unsupported service_tier to be removed, got %#v", filtered)
			}
			if filtered["temperature"] != 0.7 {
				t.Fatalf("expected other allowed options to remain, got %#v", filtered)
			}
		})
	}
}

func TestFilterModelOptionsRejectsUserOpenAIPromptCacheFields(t *testing.T) {
	for _, mode := range []string{modelOptionPolicyAllowlist, modelOptionPolicyDenylist} {
		filtered := filterModelOptions(map[string]any{
			"temperature":             0.2,
			"prompt_cache_key":        "user-controlled-key",
			"prompt_cache_options":    map[string]any{"mode": "explicit", "ttl": "30m"},
			"prompt_cache_breakpoint": map[string]any{"mode": "explicit"},
			"prompt_cache_retention":  "24h",
		}, llm.AdapterOpenAIResponses, modelOptionPolicyConfig{
			Mode:             mode,
			AllowedPathsJSON: `{"default":["temperature","prompt_cache_key","prompt_cache_options.mode","prompt_cache_options.ttl","prompt_cache_breakpoint","prompt_cache_retention"]}`,
		})
		for _, key := range []string{"prompt_cache_key", "prompt_cache_options", "prompt_cache_breakpoint", "prompt_cache_retention"} {
			if _, ok := filtered[key]; ok {
				t.Fatalf("expected %s to remain server-controlled in %s mode, got %#v", key, mode, filtered)
			}
		}
		if filtered["temperature"] != 0.2 {
			t.Fatalf("expected unrelated options to remain in %s mode, got %#v", mode, filtered)
		}
	}
}

func TestFilterModelOptionsKeepsOpenRouterChatServiceTierOutOfDefaultAllowlist(t *testing.T) {
	filtered := filterModelOptions(map[string]any{
		"service_tier":     "priority",
		"reasoning_effort": "high",
	}, llm.AdapterOpenRouterChat, modelOptionPolicyConfig{
		Mode:             modelOptionPolicyAllowlist,
		AllowedPathsJSON: config.DefaultModelOptionAllowedPathsJSON(),
		DeniedPathsJSON:  config.DefaultModelOptionDeniedPathsJSON(),
	})

	if _, ok := filtered["service_tier"]; ok {
		t.Fatalf("expected OpenRouter Chat service_tier to stay outside the default allowlist, got %#v", filtered)
	}
	if filtered["reasoning_effort"] != "high" {
		t.Fatalf("expected OpenRouter Chat reasoning_effort to pass, got %#v", filtered)
	}
}

func TestFilterModelOptionsDenylistAllowsUnlistedAndRemovesDenied(t *testing.T) {
	filtered := filterModelOptions(map[string]any{
		"temperature":          0.2,
		"custom_vendor_option": true,
		"previous_response_id": "resp_123",
		"reasoning": map[string]any{
			"effort": "high",
		},
	}, llm.AdapterXAIResponses, modelOptionPolicyConfig{
		Mode:             modelOptionPolicyDenylist,
		AllowedPathsJSON: config.DefaultModelOptionAllowedPathsJSON(),
		DeniedPathsJSON:  `{"default":["reasoning.effort"]}`,
	})

	if filtered["custom_vendor_option"] != true {
		t.Fatalf("expected custom option to pass in denylist mode, got %#v", filtered)
	}
	if _, ok := filtered["previous_response_id"]; ok {
		t.Fatalf("expected previous_response_id to be hard denied, got %#v", filtered)
	}
	if reasoning, ok := filtered["reasoning"].(map[string]any); ok {
		if _, ok := reasoning["effort"]; ok {
			t.Fatalf("expected configured deny path removed, got %#v", filtered)
		}
	}
}

func TestFilterModelOptionsOpenAIChatCompletionsAllowsThinkingType(t *testing.T) {
	filtered := filterModelOptions(map[string]any{
		"thinking": map[string]any{
			"type":          "enabled",
			"budget_tokens": 1024,
		},
	}, llm.AdapterOpenAIChatCompletions, modelOptionPolicyConfig{
		Mode:             modelOptionPolicyAllowlist,
		AllowedPathsJSON: config.DefaultModelOptionAllowedPathsJSON(),
		DeniedPathsJSON:  config.DefaultModelOptionDeniedPathsJSON(),
	})

	thinking := filtered["thinking"].(map[string]any)
	if thinking["type"] != "enabled" {
		t.Fatalf("expected thinking.type to pass for chat completions, got %#v", filtered)
	}
	if _, ok := thinking["budget_tokens"]; ok {
		t.Fatalf("expected unlisted thinking.budget_tokens to be removed for chat completions, got %#v", filtered)
	}
}

func TestPromptCarriesAssistantReasoning(t *testing.T) {
	cases := map[string]struct {
		messages []llm.Message
		want     bool
	}{
		"assistant with reasoning": {
			messages: []llm.Message{
				{Role: "user", Content: "q"},
				{Role: "assistant", Content: "a", ReasoningContent: "thinking"},
			},
			want: true,
		},
		"assistant reasoning is whitespace": {
			messages: []llm.Message{{Role: "assistant", Content: "a", ReasoningContent: "  \n "}},
			want:     false,
		},
		"reasoning on user role is ignored": {
			messages: []llm.Message{{Role: "user", Content: "q", ReasoningContent: "leaked"}},
			want:     false,
		},
		"no history": {messages: nil, want: false},
	}
	for name, item := range cases {
		if got := promptCarriesAssistantReasoning(item.messages); got != item.want {
			t.Fatalf("%s: promptCarriesAssistantReasoning() = %v, want %v", name, got, item.want)
		}
	}
}

// 首轮没有历史推理、或用户关掉回传时都不应下发厂商私有入参，否则等于白付推理 token，
// 还可能让把未知顶层字段判为非法入参的自建后端直接报错。
func TestShouldApplyReasoningPassbackRequestOptions(t *testing.T) {
	required := map[string]any{"preserve_thinking": true}
	withReasoning := []llm.Message{{Role: "assistant", Content: "a", ReasoningContent: "thinking"}}
	withoutReasoning := []llm.Message{{Role: "user", Content: "q"}}

	if !shouldApplyReasoningPassbackRequestOptions(true, required, withReasoning) {
		t.Fatal("expected injection when passback is on and history carries reasoning")
	}
	if shouldApplyReasoningPassbackRequestOptions(false, required, withReasoning) {
		t.Fatal("expected no injection when passback is disabled")
	}
	if shouldApplyReasoningPassbackRequestOptions(true, nil, withReasoning) {
		t.Fatal("expected no injection when the route requires no vendor options")
	}
	if shouldApplyReasoningPassbackRequestOptions(true, required, withoutReasoning) {
		t.Fatal("expected no injection on a first turn without historical reasoning")
	}

	// 厂商判定是按 vendor 而非按模型的：detectModelVendor 会把 wanx / qwen-vl / qwen2.5
	// 等非思考模型一并归到 alibaba，路由层因此也标记它们「需要 preserve_thinking」。
	// 这些模型不产 reasoning_content，历史推理守卫是唯一防线——它必须挡住，
	// 否则会向不认识该入参的自建后端（vLLM/Ollama 等）发送未知顶层字段。
	nonThinkingHistory := []llm.Message{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "q2"},
	}
	if shouldApplyReasoningPassbackRequestOptions(true, required, nonThinkingHistory) {
		t.Fatal("expected no injection for a non-thinking model that never emits reasoning")
	}
}

func TestWithReasoningPassbackRequestOptions(t *testing.T) {
	required := map[string]any{"preserve_thinking": true}

	got := withReasoningPassbackRequestOptions(
		map[string]any{"temperature": 0.7}, required, nil, "")
	if got["preserve_thinking"] != true || got["temperature"] != 0.7 {
		t.Fatalf("expected injection alongside existing options, got %#v", got)
	}

	// 策略模式 disabled 时过滤结果为 nil，需要新建 map 而不是 panic。
	if fromNil := withReasoningPassbackRequestOptions(nil, required, nil, ""); fromNil["preserve_thinking"] != true {
		t.Fatalf("expected a new map to be allocated, got %#v", fromNil)
	}

	if noop := withReasoningPassbackRequestOptions(nil, nil, nil, ""); noop != nil {
		t.Fatalf("expected untouched nil when nothing is required, got %#v", noop)
	}

	// 用户显式设的值不被覆盖——包括已被白名单丢掉、只存在于原始入参里的那种。
	kept := withReasoningPassbackRequestOptions(
		map[string]any{"preserve_thinking": false}, required, nil, "")
	if kept["preserve_thinking"] != false {
		t.Fatalf("expected explicit user value to survive, got %#v", kept)
	}
	rawOptions := map[string]any{"preserve_thinking": false}
	dropped := withReasoningPassbackRequestOptions(
		map[string]any{}, required, rawOptions, "")
	if _, exists := dropped["preserve_thinking"]; exists {
		t.Fatalf("expected allowlist-dropped user value to block injection, got %#v", dropped)
	}

	// 管理员在模型能力里设的默认值同样是显式意图。
	capabilities := `{"defaultOptions":{"preserve_thinking":false}}`
	fromCapabilities := withReasoningPassbackRequestOptions(
		map[string]any{}, required, nil, capabilities)
	if _, exists := fromCapabilities["preserve_thinking"]; exists {
		t.Fatalf("expected capability default to block injection, got %#v", fromCapabilities)
	}

	// 入参不得被就地修改。
	if len(required) != 1 || required["preserve_thinking"] != true {
		t.Fatalf("required map was mutated: %#v", required)
	}
	if len(rawOptions) != 1 || rawOptions["preserve_thinking"] != false {
		t.Fatalf("raw options were mutated: %#v", rawOptions)
	}
}

// 守卫扫描的是真实发往上游的 llmMessages。若历史推理在 historyMessagesFromDomain →
// cloneLLMMessages 这段链路上被丢掉，守卫会恒为 false，功能静默失效且无任何报错——
// 正是 #529 的失效形态。这里锁死该链路。
func TestReasoningPassbackGuardSeesHistoryFromDomain(t *testing.T) {
	domainMessages := []domainconversation.Message{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1", ReasoningContent: "historical thinking"},
		{Role: "user", Content: "q2"},
	}

	history := historyMessagesFromDomain(domainMessages, historyMessageOptions{ReasoningContentPassback: true})
	if !promptCarriesAssistantReasoning(history) {
		t.Fatal("guard cannot see reasoning right after historyMessagesFromDomain")
	}
	if !promptCarriesAssistantReasoning(cloneLLMMessages(history)) {
		t.Fatal("cloneLLMMessages dropped reasoning before the guard runs")
	}

	// 回传关闭时历史不带推理，守卫必须为 false，避免下发无用入参。
	disabled := historyMessagesFromDomain(domainMessages, historyMessageOptions{ReasoningContentPassback: false})
	if promptCarriesAssistantReasoning(disabled) {
		t.Fatal("guard should stay false when passback is disabled")
	}
}
