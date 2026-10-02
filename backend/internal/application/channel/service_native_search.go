package channel

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/nativetool"
)

// NativeSearchResult reports verified search support; an accepted request alone is not proof.
type NativeSearchResult struct {
	Status   string
	Protocol string
	ToolKey  string
	Reason   string
}

// ConfigureNativeSearch probes a single-source model before enabling hosted search by default.
// userID=0 is reserved for the admin endpoint. Existing tool choices are never overwritten.
func (s *Service) ConfigureNativeSearch(ctx context.Context, userID, upstreamID, routeID uint) (*NativeSearchResult, error) {
	upstream, err := s.repo.GetUpstreamByID(ctx, upstreamID)
	if err != nil {
		return nil, err
	}
	if upstream.OwnerUserID != userID {
		return nil, ErrUpstreamNotFound
	}
	row, err := s.buildProbeRouteFromBinding(ctx, upstreamID, routeID)
	if err != nil {
		return nil, err
	}
	model, err := s.repo.GetModelByID(ctx, row.PlatformModelID)
	if err != nil {
		return nil, err
	}
	if model.OwnerUserID != userID {
		return nil, ErrModelAccessDenied
	}
	result := &NativeSearchResult{Status: "skipped", Protocol: row.Protocol}
	capabilities, ok := nativeSearchConfig(model.CapabilitiesJSON)
	if !ok {
		result.Reason = "configured"
		return result, nil
	}
	if upstream.Status != "active" || model.Status != "active" || !hasModelKind(parseKinds(model.KindsJSON), modelKindChat) {
		result.Reason = "inactive_or_non_chat"
		return result, nil
	}
	sources, total, err := s.repo.ListModelUpstreamSources(ctx, model.PlatformModelName, 0, 2)
	if err != nil {
		return nil, err
	}
	if total != 1 || len(sources) != 1 || sources[0].ID != routeID || sources[0].Status != "active" || sources[0].UpstreamModelStatus != "active" {
		result.Reason = "multiple_or_inactive_routes"
		return result, nil
	}
	candidates := nativeSearchCandidates(row.Protocol, row.UpstreamModelName)
	if len(candidates) == 0 {
		result.Reason = "unsupported_protocol"
		return result, nil
	}
	if s.llmClient == nil {
		result.Reason = "client_unavailable"
		return result, nil
	}
	if failure, valid := s.prepareModelProbeRoute(*row); !valid {
		result.Status, result.Reason = "unavailable", failure.ErrorCode
		return result, nil
	}
	keyConfig, err := s.parseAPIKeysConfig(upstream.APIKeysEnc)
	if err != nil {
		return nil, err
	}
	key, err := selectProbeAPIKey(keyConfig)
	if err != nil {
		return nil, err
	}
	// Preserve strict outbound policy for user-owned upstreams, including redirected requests.
	row.UpstreamOwnerUserID = upstream.OwnerUserID
	resolved := buildResolvedRoute(*row, key)
	referer, title := s.llmAttribution()
	route := modelProbeRouteConfig(resolved, referer, title)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if s.nativeSearchProbes != nil {
		select {
		case s.nativeSearchProbes <- struct{}{}:
			defer func() { <-s.nativeSearchProbes }()
		case <-ctx.Done():
			result.Status, result.Reason = "unavailable", "timeout"
			return result, nil
		}
	}
	result.Status, result.Reason = "unavailable", "search_not_observed"
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			result.Reason = "timeout"
			break
		}
		candidateProtocol := nativeSearchAdapter(candidate)
		route.Protocol = candidateProtocol
		route.Endpoint = llm.DefaultEndpointForAdapter(candidateProtocol)
		probeCtx, probeCancel := context.WithTimeout(ctx, 20*time.Second)
		output, probeErr := s.llmClient.Generate(probeCtx, route, nativeSearchProbeInput(candidate))
		probeCancel()
		if probeErr != nil {
			result.Reason, _, _ = classifyModelProbeError(probeErr)
			if result.Reason != "request_invalid" && result.Reason != "model_not_found" {
				break
			}
			continue
		}
		if !nativeSearchObserved(output, candidate) {
			continue
		}
		config := withNativeSearch(capabilities, candidate)
		// Recheck under the same locks used by source edits; never overwrite a concurrent edit.
		err = s.repo.WithinTransaction(ctx, func(repo repository.ChannelRepository) error {
			locked, err := repo.ListModelUpstreamSourcesForUpdate(ctx, model.PlatformModelName)
			if err != nil {
				return err
			}
			current, err := repo.GetModelByID(ctx, model.ID)
			if err != nil {
				return err
			}
			currentUpstream, err := repo.GetUpstreamByID(ctx, upstreamID)
			if err != nil {
				return err
			}
			if len(locked) != 1 || locked[0].ID != routeID || locked[0].Protocol != row.Protocol || locked[0].Status != "active" || locked[0].UpstreamModelStatus != "active" || locked[0].UpstreamModelName != row.UpstreamModelName || locked[0].HeadersJSON != row.RouteHeadersJSON ||
				current.CapabilitiesJSON != model.CapabilitiesJSON || current.KindsJSON != model.KindsJSON || current.OwnerUserID != userID || current.Status != "active" ||
				currentUpstream.OwnerUserID != userID || currentUpstream.BaseURL != upstream.BaseURL || currentUpstream.APIKeysEnc != upstream.APIKeysEnc || currentUpstream.HeadersJSON != upstream.HeadersJSON || currentUpstream.Status != "active" {
				result.Status, result.Reason = "skipped", "configuration_changed"
				return nil
			}
			if err := repo.UpdatePlatformModelRouteByID(ctx, routeID, upstreamID, repository.UpdateChannelPlatformRouteInput{Protocol: &candidateProtocol}); err != nil {
				return err
			}
			if err := repo.UpdateModel(ctx, model.ID, repository.UpdateChannelModelInput{CapabilitiesJSON: &config}); err != nil {
				return err
			}
			result.Status, result.Protocol, result.ToolKey, result.Reason = "enabled", candidateProtocol, candidate.Key, ""
			return nil
		})
		if err != nil {
			return nil, err
		}
		s.InvalidateModelCatalog()
		return result, nil
	}
	return result, nil
}

func nativeSearchConfig(raw string) (map[string]any, bool) {
	config := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if json.Unmarshal([]byte(raw), &config) != nil || config == nil {
			return nil, false
		}
	}
	// Explicit empty arrays also express an intentional administrator choice.
	for _, key := range []string{"nativeTools", "nativeToolKeys"} {
		if _, exists := config[key]; exists {
			return nil, false
		}
	}
	if defaults, ok := config["defaultOptions"].(map[string]any); ok {
		for _, key := range []string{"tools", "web_search"} {
			if _, exists := defaults[key]; exists {
				return nil, false
			}
		}
	}
	return config, true
}

func nativeSearchCandidates(protocol, modelName string) []nativetool.Definition {
	// Names only suggest which native endpoint to try; support is verified by its response.
	if protocol == llm.AdapterOpenAIChatCompletions {
		name := strings.ToLower(modelName)
		switch {
		case strings.Contains(name, "claude"):
			protocol = llm.AdapterAnthropicMessages
		case strings.Contains(name, "gemini"):
			protocol = llm.AdapterGoogleGenerateContent
		case strings.Contains(name, "grok"):
			protocol = llm.AdapterXAIResponses
		case strings.Contains(name, "gpt"), strings.HasPrefix(name, "o3"), strings.HasPrefix(name, "o4"):
			protocol = llm.AdapterOpenAIResponses
		default:
			return nil
		}
	}
	keys := []string{}
	switch protocol {
	case llm.AdapterOpenAIResponses:
		keys = []string{"openai.web_search", "openai.web_search_preview"}
	case llm.AdapterAnthropicMessages:
		keys = []string{"anthropic.web_search_20250305"}
	case llm.AdapterXAIResponses:
		keys = []string{"xai.web_search"}
	case llm.AdapterGoogleGenerateContent, llm.AdapterGeminiInteractions:
		keys = []string{"google.google_search"}
	}
	result := []nativetool.Definition{}
	for _, key := range keys {
		for _, definition := range nativetool.Definitions() {
			if nativeSearchAdapter(definition) == protocol && definition.Key == key {
				result = append(result, definition)
				break
			}
		}
	}
	return result
}

func nativeSearchAdapter(tool nativetool.Definition) string {
	if tool.Protocol == "gemini_generate_content" {
		return llm.AdapterGoogleGenerateContent
	}
	return tool.Protocol
}

func nativeSearchProbeInput(tool nativetool.Definition) llm.GenerateInput {
	options := map[string]any{"tools": []map[string]any{tool.Payload}}
	switch nativeSearchAdapter(tool) {
	case llm.AdapterOpenAIResponses, llm.AdapterXAIResponses:
		options["tool_choice"] = "required"
		options["max_output_tokens"] = 512
	case llm.AdapterAnthropicMessages:
		options["max_tokens"] = 512
	case llm.AdapterGoogleGenerateContent:
		options["max_output_tokens"] = 512
	}
	return llm.GenerateInput{
		Ephemeral: true,
		Messages:  []llm.Message{{Role: "user", Content: "Use web search once to find the latest NASA news. Reply with one headline and its source URL."}},
		Options:   options,
	}
}

func nativeSearchObserved(output *llm.GenerateOutput, tool nativetool.Definition) bool {
	if output == nil {
		return false
	}
	observed := false
	for _, call := range output.ServerToolCalls {
		if call.ToolName != "web_search" && call.ToolName != "google_search" && call.ToolType != "web_search_call" && call.ToolType != "google_search_call" {
			continue
		}
		if call.ErrorJSON != "" || call.Status == "error" || call.Status == "failed" {
			return false
		}
		if call.Status == "completed" {
			observed = true
		}
	}
	// Gemini's adapter exposes verified grounding metadata as server-side tool usage.
	if tool.Key == "google.google_search" && output.ServerSideToolUsage["google_search"] > 0 {
		observed = true
	}
	return observed
}

func withNativeSearch(config map[string]any, tool nativetool.Definition) string {
	defaults, _ := config["defaultOptions"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}
	defaults["tools"] = []map[string]any{tool.Payload}
	config["defaultOptions"] = defaults
	config["nativeTools"] = []map[string]any{{
		"key": tool.Key, "protocols": []string{tool.Protocol}, "type": tool.Type,
		"enabled": true, "defaultEnabled": true, "payload": tool.Payload,
	}}
	payload, _ := json.Marshal(config)
	return string(payload)
}
