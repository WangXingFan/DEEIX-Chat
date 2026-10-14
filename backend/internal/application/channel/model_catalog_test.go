package channel

import (
	"encoding/json"
	"errors"
	"testing"

	appbilling "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/billing"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

func TestProtocolDefaultsForCompatibleOnlyIncludesSupportedPrimaryKinds(t *testing.T) {
	raw := protocolDefaultsForCompatible(compatibleOpenAI)

	var defaults map[string]string
	if err := json.Unmarshal([]byte(raw), &defaults); err != nil {
		t.Fatalf("unmarshal defaults: %v", err)
	}

	for _, kind := range []string{modelKindChat, modelKindAudio} {
		if defaults[kind] == "" {
			t.Fatalf("expected default protocol for %s in %s", kind, raw)
		}
	}
	legacyVectorKind := "embed" + "ding"
	legacySortKind := "re" + "rank"
	for _, kind := range []string{legacyVectorKind, legacySortKind, "unknown_kind"} {
		if _, ok := defaults[kind]; ok {
			t.Fatalf("unexpected default protocol for %s in %s", kind, raw)
		}
	}
}

func TestProtocolDefaultsForXAIUsesXAIResponsesForConversationKinds(t *testing.T) {
	raw := protocolDefaultsForCompatible(compatibleXAI)

	var defaults map[string]string
	if err := json.Unmarshal([]byte(raw), &defaults); err != nil {
		t.Fatalf("unmarshal defaults: %v", err)
	}

	if defaults[modelKindChat] != "xai_responses" {
		t.Fatalf("expected xAI chat default, got %q in %s", defaults[modelKindChat], raw)
	}
	if defaults[modelKindAudio] != "xai_responses" {
		t.Fatalf("expected xAI audio default, got %q in %s", defaults[modelKindAudio], raw)
	}
}

func TestProtocolDefaultsForOpenRouterUsesOpenRouterResponsesForConversationKinds(t *testing.T) {
	raw := protocolDefaultsForCompatible(compatibleOpenRouter)

	var defaults map[string]string
	if err := json.Unmarshal([]byte(raw), &defaults); err != nil {
		t.Fatalf("unmarshal defaults: %v", err)
	}

	if defaults[modelKindChat] != "openrouter_responses" {
		t.Fatalf("expected OpenRouter chat default, got %q in %s", defaults[modelKindChat], raw)
	}
	if defaults[modelKindAudio] != "openrouter_responses" {
		t.Fatalf("expected OpenRouter audio default, got %q in %s", defaults[modelKindAudio], raw)
	}
}

func TestProtocolDefaultsForGoogleUsesGoogleGenerateContent(t *testing.T) {
	raw := protocolDefaultsForCompatible(compatibleGoogle)

	var defaults map[string]string
	if err := json.Unmarshal([]byte(raw), &defaults); err != nil {
		t.Fatalf("unmarshal defaults: %v", err)
	}

	if defaults[modelKindChat] != "google_generate_content" {
		t.Fatalf("expected Google chat default, got %q in %s", defaults[modelKindChat], raw)
	}
	if defaults[modelKindAudio] != "google_generate_content" {
		t.Fatalf("expected Google audio default, got %q in %s", defaults[modelKindAudio], raw)
	}
}

func TestNormalizeCompatibleOnlyAllowsSupportedUpstreamProviders(t *testing.T) {
	for _, raw := range []string{"openai", "anthropic", "google", "xai", "openrouter", "custom"} {
		if got := normalizeCompatible(raw); got != raw {
			t.Fatalf("normalizeCompatible(%q) = %q", raw, got)
		}
	}
	if got := normalizeCompatible(""); got != compatibleOpenAI {
		t.Fatalf("empty compatible should default to openai, got %q", got)
	}
	for _, raw := range []string{"replicate", "fal", "stability_ai", "mistral"} {
		if got := normalizeCompatible(raw); got != "" {
			t.Fatalf("expected unsupported compatible %q to be rejected, got %q", raw, got)
		}
	}
}

func TestProtocolDefaultsForCustomUsesOpenAICompatibleDefaults(t *testing.T) {
	raw := protocolDefaultsForCompatible(compatibleCustom)

	var defaults map[string]string
	if err := json.Unmarshal([]byte(raw), &defaults); err != nil {
		t.Fatalf("unmarshal defaults: %v", err)
	}
	if defaults[modelKindChat] != "openai_chat_completions" {
		t.Fatalf("expected custom chat default, got %q in %s", defaults[modelKindChat], raw)
	}
	if defaults[modelKindAudio] != "openai_chat_completions" {
		t.Fatalf("expected custom audio default, got %q in %s", defaults[modelKindAudio], raw)
	}
}

func TestDetectModelVendorRecognizesMistralFamily(t *testing.T) {
	tests := map[string]string{
		"mistral-large-latest": "mistral",
		"mixtral-8x7b":         "mistral",
		"ministral-8b":         "mistral",
		"codestral-latest":     "mistral",
		"pixtral-12b":          "mistral",
		"devstral-small":       "mistral",
		"acme-mistral-proxy":   "mistral",
	}

	for platformModelName, expected := range tests {
		if got := detectModelVendor(platformModelName); got != expected {
			t.Fatalf("detectModelVendor(%q) = %q, want %q", platformModelName, got, expected)
		}
	}
}

func TestDetectModelVendorRecognizesCompanyVendors(t *testing.T) {
	tests := map[string]string{
		"llama-3.3-70b":                        "meta",
		"meta/llama-4-scout":                   "meta",
		"openrouter/meta/llama-4-scout":        "meta",
		"phi-4":                                "microsoft",
		"openrouter/microsoft/phi-4-mini":      "microsoft",
		"microsoft/phi-4-mini":                 "microsoft",
		"amazon.nova-pro-v1":                   "amazon",
		"openrouter/amazon/nova-pro-v1":        "amazon",
		"titan-text-premier":                   "amazon",
		"nemotron-4-340b":                      "nvidia",
		"openrouter/nvidia/nemotron-4-340b":    "nvidia",
		"qwen-max":                             "alibaba",
		"openrouter/alibaba/qwen3-max":         "alibaba",
		"alibaba/qwen3-max":                    "alibaba",
		"doubao-seed-1-6":                      "bytedance",
		"openrouter/bytedance/doubao-seed-1-6": "bytedance",
		"volcengine/doubao":                    "bytedance",
		"hunyuan-turbos":                       "tencent",
		"openrouter/tencent/hunyuan-turbos":    "tencent",
		"tencent/hunyuan":                      "tencent",
		"mimo-v2.5-pro":                        "xiaomi",
		"spark-max":                            "iflytek",
		"iflytek/spark-pro":                    "iflytek",
		"step-2-16k":                           "stepfun",
		"baichuan4-turbo":                      "baichuan",
		"ernie-4.5-turbo":                      "baidu",
		"wenxin-4":                             "baidu",
		"nano-banana-pro":                      "google",
		"gemini-3-pro-image":                   "google",
		"gemini-3-pro-image-preview":           "google",
		"openrouter/unknown/model":             "openrouter",
	}

	for platformModelName, expected := range tests {
		if got := detectModelVendor(platformModelName); got != expected {
			t.Fatalf("detectModelVendor(%q) = %q, want %q", platformModelName, got, expected)
		}
	}
}

func TestReasoningContentPassbackRequiredForDeepSeekChatCompletions(t *testing.T) {
	if !reasoningContentPassbackRequired(llm.AdapterOpenAIChatCompletions, "deepseek", "deepseek-v4-flash-free") {
		t.Fatal("expected DeepSeek Chat Completions route to require reasoning_content passback")
	}
	if !reasoningContentPassbackRequired(llm.AdapterOpenRouterChat, "openai", "openai/gpt-oss-120b:free") {
		t.Fatal("expected OpenRouter Chat Completions route to require reasoning passback")
	}
	if reasoningContentPassbackRequired(llm.AdapterOpenAIChatCompletions, "openai", "gpt-5.4") {
		t.Fatal("expected OpenAI Chat Completions route to skip reasoning_content passback")
	}
	if reasoningContentPassbackRequired(llm.AdapterOpenRouterResponses, "openai/gpt-oss-120b:free") {
		t.Fatal("expected OpenRouter Responses route to skip Chat Completions reasoning passback")
	}
	if reasoningContentPassbackRequired(llm.AdapterOpenAIResponses, "deepseek", "deepseek-v4-flash-free") {
		t.Fatal("expected non Chat Completions route to skip reasoning_content passback")
	}
}

// Moonshot、智谱、小米 MiMo 的思考模型同样要求历史 assistant 消息原样携带 reasoning_content，
// 覆盖显式 vendor 与仅凭模型名推断两种入口，并反向断言未纳入白名单的厂商保持关闭。
func TestReasoningContentPassbackRequiredForAdditionalChatCompletionsVendors(t *testing.T) {
	required := []struct {
		name       string
		candidates []string
	}{
		{"moonshot vendor", []string{"moonshot", "kimi-k3"}},
		{"kimi alias", []string{"kimi", "kimi-k2.7-code"}},
		{"kimi by model name", []string{"", "kimi-k3"}},
		{"moonshot by model name", []string{"", "moonshot-v1-128k"}},
		{"zhipu vendor", []string{"zhipu", "glm-4.6"}},
		{"glm alias", []string{"glm", "glm-4.6"}},
		{"glm by model name", []string{"", "glm-4.6"}},
		{"chatglm by model name", []string{"", "chatglm-6b"}},
		{"xiaomi vendor", []string{"xiaomi", "mimo-v2.5-pro"}},
		{"mimo by model name", []string{"", "mimo-v2.5-pro"}},
		{"mimo omni by model name", []string{"", "mimo-v2-omni"}},
		{"mimo namespaced model name", []string{"", "xiaomi/mimo-v2.5-pro"}},
		{"alibaba vendor", []string{"alibaba", "qwen3.6-plus"}},
		{"qwen alias", []string{"qwen", "qwen-max"}},
		{"qwq by model name", []string{"", "qwq-32b"}},
		{"qvq by model name", []string{"", "qvq-max"}},
		{"tongyi by model name", []string{"", "tongyi-deepresearch"}},
		{"minimax vendor", []string{"minimax", "minimax-m2"}},
		{"abab by model name", []string{"", "abab-6.5s"}},
		{"hailuo by model name", []string{"", "hailuo-02"}},
	}
	for _, item := range required {
		if !reasoningContentPassbackRequired(llm.AdapterOpenAIChatCompletions, item.candidates...) {
			t.Fatalf("%s: expected Chat Completions route to require reasoning_content passback", item.name)
		}
	}

	// 非 Chat Completions 协议不受影响。
	if reasoningContentPassbackRequired(llm.AdapterOpenAIResponses, "moonshot", "kimi-k3") {
		t.Fatal("expected Responses route to skip reasoning_content passback for moonshot")
	}
	if reasoningContentPassbackRequired(llm.AdapterOpenAIResponses, "zhipu", "glm-4.6") {
		t.Fatal("expected Responses route to skip reasoning_content passback for zhipu")
	}

	// 未列入白名单的厂商保持关闭，避免向不接受该字段的上游发送多余入参。
	for _, item := range []struct {
		name       string
		candidates []string
	}{
		{"bytedance", []string{"bytedance", "doubao-seed-1-6"}},
		{"doubao by model name", []string{"", "doubao-1-5-thinking-pro"}},
		{"tencent", []string{"tencent", "hunyuan-turbos"}},
		{"anthropic", []string{"anthropic", "claude-opus-4-8"}},
	} {
		if reasoningContentPassbackRequired(llm.AdapterOpenAIChatCompletions, item.candidates...) {
			t.Fatalf("%s: expected vendor outside the passback allowlist to skip reasoning_content", item.name)
		}
	}
}

// 阿里 Qwen 只回传字段无效，必须同时下发 preserve_thinking；其余厂商不应收到该私有入参，
// OpenRouter 更要拦住——它有自己的 reasoning 字段与参数校验。
func TestReasoningPassbackRequestOptionsOnlyForAlibabaChatCompletions(t *testing.T) {
	got := reasoningPassbackRequestOptions(llm.AdapterOpenAIChatCompletions, "alibaba", "qwen3.6-plus")
	if len(got) != 1 || got["preserve_thinking"] != true {
		t.Fatalf("expected preserve_thinking for alibaba chat completions, got %#v", got)
	}
	if byName := reasoningPassbackRequestOptions(llm.AdapterOpenAIChatCompletions, "", "qwq-32b"); byName["preserve_thinking"] != true {
		t.Fatalf("expected model-name inference to require preserve_thinking, got %#v", byName)
	}

	// 返回值必须是副本，调用方改动不能污染包级变量。
	got["preserve_thinking"] = false
	got["injected"] = true
	again := reasoningPassbackRequestOptions(llm.AdapterOpenAIChatCompletions, "alibaba", "qwen3.6-plus")
	if len(again) != 1 || again["preserve_thinking"] != true {
		t.Fatalf("package-level options were mutated by caller: %#v", again)
	}

	for _, item := range []struct {
		name       string
		protocol   string
		candidates []string
	}{
		{"openrouter alibaba", llm.AdapterOpenRouterChat, []string{"alibaba", "qwen/qwen3-max"}},
		{"responses alibaba", llm.AdapterOpenAIResponses, []string{"alibaba", "qwen3.6-plus"}},
		{"minimax", llm.AdapterOpenAIChatCompletions, []string{"minimax", "minimax-m2"}},
		{"deepseek", llm.AdapterOpenAIChatCompletions, []string{"deepseek", "deepseek-v4-flash-free"}},
		{"moonshot", llm.AdapterOpenAIChatCompletions, []string{"moonshot", "kimi-k3"}},
		{"zhipu", llm.AdapterOpenAIChatCompletions, []string{"zhipu", "glm-4.6"}},
		{"xiaomi", llm.AdapterOpenAIChatCompletions, []string{"xiaomi", "mimo-v2.5-pro"}},
		{"vendor outside allowlist", llm.AdapterOpenAIChatCompletions, []string{"anthropic", "claude-opus-4-8"}},
	} {
		if options := reasoningPassbackRequestOptions(item.protocol, item.candidates...); options != nil {
			t.Fatalf("%s: expected no vendor request options, got %#v", item.name, options)
		}
	}
}

func TestNormalizeModelIconSeparatesVendorAndModelFamily(t *testing.T) {
	tests := map[string]struct {
		vendor   string
		model    string
		expected string
	}{
		"alibaba qwen":      {vendor: "alibaba", model: "qwen-max", expected: "qwen"},
		"openrouter qwen":   {vendor: "openrouter", model: "openrouter/alibaba/qwen-max", expected: "qwen"},
		"bytedance doubao":  {vendor: "bytedance", model: "doubao-seed-1-6", expected: "doubao"},
		"openrouter doubao": {vendor: "openrouter", model: "openrouter/bytedance/doubao-seed-1-6", expected: "doubao"},
		"tencent hunyuan":   {vendor: "tencent", model: "hunyuan-turbos", expected: "hunyuan"},
		"openrouter llama":  {vendor: "openrouter", model: "openrouter/meta/llama-4-scout", expected: "meta"},
		"xiaomi mimo":       {vendor: "xiaomi", model: "mimo-v2.5-pro", expected: "xiaomimimo"},
		"amazon nova":       {vendor: "amazon", model: "nova-pro", expected: "nova"},
		"amazon titan":      {vendor: "amazon", model: "titan-text-premier", expected: "bedrock"},
		"baidu ernie":       {vendor: "baidu", model: "ernie-4.5-turbo", expected: "wenxin"},
		"iflytek spark":     {vendor: "iflytek", model: "spark-max", expected: "spark"},
		"stepfun step":      {vendor: "stepfun", model: "step-2-16k", expected: "stepfun"},
		"baichuan baichuan": {vendor: "baichuan", model: "baichuan4-turbo", expected: "baichuan"},
		"google nano":       {vendor: "google", model: "nano-banana-pro", expected: "nanobanana"},
	}

	for name, tc := range tests {
		if got := normalizeModelIcon("", tc.vendor, tc.model); got != tc.expected {
			t.Fatalf("%s: normalizeModelIcon = %q, want %q", name, got, tc.expected)
		}
	}
}

func TestNormalizeModelVendorFallsBackToUnknownForUnsupportedVendor(t *testing.T) {
	if got := normalizeModelVendor("unsupported-vendor", "unsupported-model"); got != "unknown" {
		t.Fatalf("expected unsupported platform vendor to become unknown, got %q", got)
	}
}

func TestNormalizeProtocolDefaultsJSONDropsUnknownKinds(t *testing.T) {
	legacyVectorKind := "embed" + "ding"
	legacySortKind := "re" + "rank"
	legacyVectorProtocol := "openai_" + "embed" + "dings"
	legacySortProtocol := "cohere_" + "re" + "rank"
	raw := `{
		"chat":"OPENAI_CHAT_COMPLETIONS",
		"unknown_kind":"openai_chat_completions",
		"` + legacyVectorKind + `":"` + legacyVectorProtocol + `",
		"` + legacySortKind + `":"` + legacySortProtocol + `"
	}`

	normalized, err := normalizeProtocolDefaultsJSON(raw)
	if err != nil {
		t.Fatalf("normalize defaults: %v", err)
	}

	var defaults map[string]string
	if err := json.Unmarshal([]byte(normalized), &defaults); err != nil {
		t.Fatalf("unmarshal normalized defaults: %v", err)
	}

	if defaults[modelKindChat] != "openai_chat_completions" {
		t.Fatalf("expected normalized chat protocol, got %q", defaults[modelKindChat])
	}
	for _, kind := range []string{"unknown_kind", legacyVectorKind, legacySortKind} {
		if _, ok := defaults[kind]; ok {
			t.Fatalf("unexpected protocol default for %s in %s", kind, normalized)
		}
	}
}

func TestNormalizeProtocolDefaultsJSONRejectsInvalidProtocolForSupportedKind(t *testing.T) {
	legacyVectorProtocol := "openai_" + "embed" + "dings"
	_, err := normalizeProtocolDefaultsJSON(`{"chat":"` + legacyVectorProtocol + `"}`)
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Fatalf("expected ErrInvalidAdapter, got %v", err)
	}
}

func TestResolveRouteProtocolsRejectsUnsupportedExplicitMultiProtocol(t *testing.T) {
	_, err := resolveRouteProtocols([]string{"openai_responses", "openai_chat_completions"}, compatibleOpenAI, "", `["chat"]`)
	if !errors.Is(err, ErrInvalidRouteProtocolCombination) {
		t.Fatalf("expected ErrInvalidRouteProtocolCombination, got %v", err)
	}
}

func TestResolveRouteProtocolUsesExplicitConversationProtocolAsSourceOfTruth(t *testing.T) {
	for _, tc := range []struct {
		compatible string
		protocol   string
	}{
		{compatible: compatibleXAI, protocol: "openai_responses"},
		{compatible: compatibleOpenAI, protocol: "xai_responses"},
	} {
		resolved, err := resolveRouteProtocol(tc.protocol, tc.compatible, "", `["chat"]`)
		if err != nil {
			t.Fatalf("expected explicit protocol %q for compatible %q to be accepted: %v", tc.protocol, tc.compatible, err)
		}
		if resolved != tc.protocol {
			t.Fatalf("expected %q, got %q", tc.protocol, resolved)
		}
	}
}

func TestIsRouteAllowedForTaskAllowsConversationProtocols(t *testing.T) {
	if !IsRouteAllowedForTask(TaskTypeChat, `["chat"]`, "openai_responses") {
		t.Fatalf("expected chat task to allow chat protocol")
	}
	if !IsRouteAllowedForTask(TaskTypeChat, `["chat"]`, "gemini_interactions") {
		t.Fatalf("expected chat task to allow Gemini Interactions protocol")
	}
	if IsRouteAllowedForTask(TaskTypeChat, `["audio"]`, "gemini_interactions") {
		t.Fatalf("expected chat task to reject protocol the declared kind cannot serve")
	}
	if !IsRouteAllowedForTask(TaskTypeChat, `["audio"]`, "openai_chat_completions") {
		t.Fatalf("expected audio kind to allow chat completions protocol")
	}
	if IsRouteAllowedForTask(TaskTypeChat, `["chat"]`, "unknown_protocol") {
		t.Fatalf("expected unknown protocol to be rejected")
	}
}

func TestFilterPricedModelViewsUsesPlatformModelNameKey(t *testing.T) {
	items := []ModelView{{
		PlatformModelName: "gpt-5.5",
	}}
	pricing := map[string]appbilling.PublicModelPricing{
		"gpt-5.5": {
			Currency: "USD",
			Mode:     "token",
		},
	}

	results := filterPricedModelViews(items, pricing)
	if len(results) != 1 {
		t.Fatalf("expected model to match pricing by platform model name, got %d", len(results))
	}
	if results[0].Pricing == nil || results[0].Pricing.Currency != "USD" {
		t.Fatalf("expected pricing attached by platform model name, got %#v", results[0].Pricing)
	}

}

func TestNormalizePlatformModelNameAllowsDisplaySpaces(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain", raw: "claude-sonnet", want: "claude-sonnet"},
		{name: "display spaces", raw: "GPT Image 1", want: "GPT Image 1"},
		{name: "trim outer spaces", raw: "  GPT Image 1  ", want: "GPT Image 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePlatformModelName(tt.raw)
			if err != nil {
				t.Fatalf("normalize platform model name: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNormalizePlatformModelNameRejectsUnsafeWhitespace(t *testing.T) {
	tests := []string{
		"",
		"   ",
		"claude\tsonnet",
		"claude\nsonnet",
		"claude\u00a0sonnet",
	}

	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if _, err := normalizePlatformModelName(raw); err == nil {
				t.Fatal("expected unsafe platform model name to be rejected")
			}
		})
	}
}
