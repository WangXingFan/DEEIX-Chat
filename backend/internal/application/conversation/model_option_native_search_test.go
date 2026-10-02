package conversation

import "testing"

func TestDetectedNativeSearchDefaultsCanBeDisabledByUser(t *testing.T) {
	cfg := modelOptionPolicyConfig{ModelCapabilitiesJSON: `{
		"defaultOptions":{"tools":[{"type":"web_search"}]},
		"nativeTools":[{"key":"openai.web_search","protocols":["openai_responses"],"type":"web_search","enabled":true,"defaultEnabled":true,"payload":{"type":"web_search"}}]
	}`}
	defaults := filterModelOptions(nil, "openai_responses", cfg)
	if defaults["tools"] == nil {
		t.Fatal("verified native search must be on without user options")
	}
	disabled := filterModelOptions(map[string]any{"tools": []any{}}, "openai_responses", cfg)
	if disabled["tools"] != nil {
		t.Fatal("explicitly turning search off must override defaults")
	}
}
