package channel

import (
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

func TestNativeSearchCandidatesNeverUseChatCompletionsToolPayloads(t *testing.T) {
	for _, model := range []string{"gpt-4.1", "claude-sonnet-4", "gemini-2.5-flash", "grok-4", "o3"} {
		candidates := nativeSearchCandidates(llm.AdapterOpenAIChatCompletions, model)
		if len(candidates) == 0 {
			t.Fatalf("missing candidates for %s", model)
		}
		for _, candidate := range candidates {
			if candidate.Protocol == llm.AdapterOpenAIChatCompletions {
				t.Fatal("native search sent to Chat Completions")
			}
		}
	}
	for _, protocol := range []string{llm.AdapterOpenAIChatCompletions, llm.AdapterOpenAIImageGenerations} {
		if len(nativeSearchCandidates(protocol, "unknown")) != 0 {
			t.Fatal("unrecognized models should remain unchanged")
		}
	}
}

func TestNativeSearchRequiresCompletedServerToolOrGoogleGrounding(t *testing.T) {
	tool := nativeSearchCandidates(llm.AdapterGoogleGenerateContent, "gemini")[0]
	if nativeSearchObserved(&llm.GenerateOutput{ToolCalls: []llm.ToolCall{{ToolName: "google_search", Status: "completed"}}}, tool) {
		t.Fatal("client tool is not hosted search")
	}
	if !nativeSearchObserved(&llm.GenerateOutput{ServerSideToolUsage: map[string]int64{"google_search": 1}}, tool) {
		t.Fatal("expected grounded Google search")
	}
	if nativeSearchObserved(&llm.GenerateOutput{
		ServerSideToolUsage: map[string]int64{"google_search": 1},
		ServerToolCalls:     []llm.ToolCall{{ToolName: "google_search", Status: "error"}},
	}, tool) {
		t.Fatal("tool error must override usage")
	}
}
