package channel_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	appchannel "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	channelrepo "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/postgres/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

type searchGateway struct {
	calls    int
	generate func(llm.RouteConfig, llm.GenerateInput) (*llm.GenerateOutput, error)
}

func (g *searchGateway) Generate(_ context.Context, route llm.RouteConfig, input llm.GenerateInput) (*llm.GenerateOutput, error) {
	g.calls++
	return g.generate(route, input)
}

func (*searchGateway) ListModels(context.Context, llm.RouteConfig) ([]llm.ModelItem, error) {
	return nil, nil
}

func TestNativeSearchConfigurationVerifiesExecutionAndPreservesChat(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		expected string
		calls    int
	}{
		{"search", "enabled", 1},
		{"google", "enabled", 1},
		{"claude", "enabled", 1},
		{"grok", "enabled", 1},
		{"ignored", "unavailable", 2},
		{"tool_error", "unavailable", 2},
		{"preview_only", "enabled", 2},
		{"auth_error", "unavailable", 1},
		{"configured", "skipped", 0},
		{"multiple_routes", "skipped", 0},
		{"concurrent_edit", "skipped", 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			modelName, expectedProtocol := "gpt-4.1", "openai_responses"
			switch scenario.name {
			case "google":
				modelName, expectedProtocol = "gemini-2.5-flash", "google_generate_content"
			case "claude":
				modelName, expectedProtocol = "claude-sonnet-4", "anthropic_messages"
			case "grok":
				modelName, expectedProtocol = "grok-4", "xai_responses"
			}
			db := openUserUpstreamTestDB(t)
			repo := channelrepo.NewRepo(db)
			gateway := &searchGateway{}
			service := appchannel.NewServiceWithRuntime(config.NewRuntime(config.Config{DataEncryptionKey: "native-search-test-key"}), repo, repo, nil, gateway)
			upstream, err := service.CreateUserUpstream(t.Context(), 7, appchannel.CreateUpstreamInput{
				Name: "New API", BaseURL: "https://example.com/v1", Compatible: "openai", APIKeys: `{"keys":[{"key":"sk-test","status":"active"}]}`, Status: "active",
			})
			if err != nil {
				t.Fatal(err)
			}
			imported, err := service.ImportUserModels(t.Context(), 7, upstream.ID, []string{modelName})
			if err != nil {
				t.Fatal(err)
			}
			binding := imported[0]
			if scenario.name == "multiple_routes" {
				var extra models.LLMPlatformModelRoute
				if err := db.First(&extra, binding.RouteID).Error; err != nil {
					t.Fatal(err)
				}
				extra.ID = 0
				extra.Protocol = "anthropic_messages"
				if err := db.Create(&extra).Error; err != nil {
					t.Fatal(err)
				}
			}
			initial := `{"defaultOptions":{"temperature":0.2},"optionControls":[]}`
			if scenario.name == "configured" {
				initial = `{"nativeToolKeys":[]}`
			}
			if err := db.Model(&models.LLMPlatformModel{}).Where("id = ?", binding.PlatformModelID).Update("capabilities_json", initial).Error; err != nil {
				t.Fatal(err)
			}
			gateway.generate = func(route llm.RouteConfig, input llm.GenerateInput) (*llm.GenerateOutput, error) {
				if !route.UserConfigured || route.Protocol != expectedProtocol || route.UpstreamModel != modelName || route.APIKey != "sk-test" {
					t.Fatalf("incorrect or unsafe probe route: protocol=%s userConfigured=%v", route.Protocol, route.UserConfigured)
				}
				if !input.Ephemeral || input.DisableTools || len(input.Tools) != 0 {
					t.Fatal("probe must be ephemeral with only hosted tools")
				}
				tools := input.Options["tools"].([]map[string]any)
				if scenario.name == "google" {
					if tools[0]["google_search"] == nil {
						t.Fatal("missing Google grounding tool")
					}
					return &llm.GenerateOutput{ServerSideToolUsage: map[string]int64{"google_search": 1}}, nil
				}
				if scenario.name == "auth_error" {
					return nil, &llm.UpstreamError{StatusCode: 401}
				}
				if scenario.name == "preview_only" && gateway.calls == 1 {
					return nil, &llm.UpstreamError{StatusCode: 400}
				}
				if scenario.name == "preview_only" && tools[0]["type"] != "web_search_preview" {
					t.Fatal("expected preview fallback")
				}
				if scenario.name == "ignored" {
					return &llm.GenerateOutput{Text: "I searched the web", Citations: []string{"https://example.com"}}, nil
				}
				if scenario.name == "concurrent_edit" {
					if err := db.Model(&models.LLMPlatformModel{}).Where("id = ?", binding.PlatformModelID).Update("capabilities_json", `{"nativeToolKeys":[]}`).Error; err != nil {
						t.Fatal(err)
					}
				}
				call := llm.ToolCall{ToolType: "web_search_call", ToolName: "web_search", Status: "completed"}
				if scenario.name == "tool_error" {
					call.Status = "error"
					call.ErrorJSON = `{"error":"not_enabled"}`
				}
				return &llm.GenerateOutput{ServerToolCalls: []llm.ToolCall{call}}, nil
			}
			// Neither another user nor an admin request may probe a private upstream.
			for _, userID := range []uint{0, 8} {
				if _, err := service.ConfigureNativeSearch(t.Context(), userID, upstream.ID, binding.RouteID); !errors.Is(err, appchannel.ErrUpstreamNotFound) {
					t.Fatalf("foreign probe: %v", err)
				}
			}
			result, err := service.ConfigureNativeSearch(t.Context(), 7, upstream.ID, binding.RouteID)
			if err != nil || result.Status != scenario.expected || gateway.calls != scenario.calls {
				t.Fatalf("result=%+v err=%v calls=%d", result, err, gateway.calls)
			}
			stored, err := repo.GetModelByID(t.Context(), binding.PlatformModelID)
			if err != nil {
				t.Fatal(err)
			}
			route, err := repo.GetPlatformModelRouteByID(t.Context(), binding.RouteID, upstream.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario.expected == "enabled" {
				if route.Protocol != expectedProtocol {
					t.Fatalf("search route not saved: %s", route.Protocol)
				}
				var capabilities struct {
					DefaultOptions map[string]any `json:"defaultOptions"`
					NativeTools    []struct {
						DefaultEnabled bool `json:"defaultEnabled"`
					} `json:"nativeTools"`
				}
				if err := json.Unmarshal([]byte(stored.CapabilitiesJSON), &capabilities); err != nil {
					t.Fatal(err)
				}
				if len(capabilities.NativeTools) != 1 || !capabilities.NativeTools[0].DefaultEnabled || capabilities.DefaultOptions["tools"] == nil || capabilities.DefaultOptions["temperature"] != 0.2 {
					t.Fatalf("search defaults or existing settings lost: %s", stored.CapabilitiesJSON)
				}
				if _, err := service.ConfigureNativeSearch(t.Context(), 7, upstream.ID, binding.RouteID); err != nil || gateway.calls != scenario.calls {
					t.Fatal("configured model must not be probed again")
				}
				reimported, err := service.ImportUserModels(t.Context(), 7, upstream.ID, []string{modelName})
				if err != nil || reimported[0].Protocol != expectedProtocol {
					t.Fatalf("reimport reset verified search protocol: %v", err)
				}
			} else {
				if route.Protocol != "openai_chat_completions" {
					t.Fatal("failed probe changed the chat route")
				}
				if scenario.name != "concurrent_edit" && stored.CapabilitiesJSON != initial {
					t.Fatal("failed probe changed capabilities")
				}
			}
		})
	}
}
