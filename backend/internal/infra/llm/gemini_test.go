package llm

import (
	"errors"
	"net/http"
	"testing"

	portllm "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

func TestParseGeminiResponseReasoningAndCitations(t *testing.T) {
	result, err := parseGeminiResponse([]byte(`{
		"responseId": "gemini-response-1",
		"candidates": [
			{
				"content": {
					"parts": [
						{"text": "internal reasoning", "thought": true, "thoughtSignature": "sig-1"},
						{"text": "final answer"}
					]
				},
				"groundingMetadata": {
					"groundingChunks": [
						{"web": {"uri": "https://example.com/a", "title": "A"}},
						{"retrievedContext": {"uri": "https://example.com/b"}}
					]
				},
				"urlContextMetadata": {
					"urlMetadata": [
						{"retrievedUrl": "https://example.com/c"}
					]
				}
			}
		]
	}`))
	if err != nil {
		t.Fatalf("parse gemini response: %v", err)
	}
	if result.Text != "final answer" {
		t.Fatalf("expected final answer without thought text, got %#v", result.Text)
	}
	if result.Reasoning == nil || result.Reasoning.Text != "internal reasoning" || result.Reasoning.Signature != "sig-1" {
		t.Fatalf("expected Gemini reasoning output, got %#v", result.Reasoning)
	}
	if len(result.Citations) != 3 || result.Citations[0] != "https://example.com/a" || result.Citations[2] != "https://example.com/c" {
		t.Fatalf("expected Gemini citations, got %#v", result.Citations)
	}
}

func TestApplyGeminiStreamChunkStoresReasoningAndCitations(t *testing.T) {
	result := &portllm.GenerateOutput{ToolCalls: make([]portllm.ToolCall, 0)}
	var reasoningText string
	err := applyGeminiStreamChunk(mustDecodeObject(t, `{
		"responseId": "gemini-stream-1",
		"candidates": [
			{
				"content": {
					"parts": [
						{"text": "think", "thought": true, "thoughtSignature": "sig-stream"},
						{"text": "answer"}
					]
				},
				"groundingMetadata": {
					"groundingChunks": [
						{"web": {"uri": "https://example.com/source"}}
					]
				}
			}
		]
	}`), result, func(event portllm.GenerateStreamEvent) error {
		if event.Reasoning != nil {
			reasoningText += event.Reasoning.Text
		}
		return nil
	})
	if err != nil {
		t.Fatalf("apply gemini stream chunk: %v", err)
	}
	if result.ResponseID != "gemini-stream-1" || result.Text != "answer" {
		t.Fatalf("expected response id and answer text, got %#v", result)
	}
	if reasoningText != "think" || result.Reasoning == nil || result.Reasoning.Text != "think" || result.Reasoning.Signature != "sig-stream" {
		t.Fatalf("expected stored and emitted reasoning, got text=%q result=%#v", reasoningText, result.Reasoning)
	}
	if len(result.Citations) != 1 || result.Citations[0] != "https://example.com/source" {
		t.Fatalf("expected stream citations, got %#v", result.Citations)
	}
}

func TestParseGeminiResponseCapturesFunctionCallThoughtSignature(t *testing.T) {
	output, err := parseGeminiResponse([]byte(`{
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"name": "search_web",
						"args": {"query": "SpaceX stock price"}
					},
					"thoughtSignature": "thought-signature-1"
				}]
			}
		}]
	}`))
	if err != nil {
		t.Fatalf("parse Gemini response: %v", err)
	}
	if len(output.ToolCalls) != 1 {
		t.Fatalf("expected one function call, got %#v", output.ToolCalls)
	}
	if output.ToolCalls[0].ThoughtSignature != "thought-signature-1" {
		t.Fatalf("expected thought signature, got %#v", output.ToolCalls[0])
	}
}

func TestParseGeminiResponseCapturesGoogleSearchUsage(t *testing.T) {
	output, err := parseGeminiResponse([]byte(`{
		"candidates": [{
			"content": {"parts": [{"text": "answer"}]},
			"groundingMetadata": {
				"webSearchQueries": ["latest docs"],
				"groundingChunks": [{"web": {"uri": "https://example.com"}}]
			}
		}]
	}`))
	if err != nil {
		t.Fatalf("parse Gemini response: %v", err)
	}
	if output.ServerSideToolUsage["google_search"] != 1 {
		t.Fatalf("expected google_search server-side usage, got %#v", output.ServerSideToolUsage)
	}
	if len(output.ServerToolCalls) != 1 {
		t.Fatalf("expected google_search server-side tool trace, got %#v", output.ServerToolCalls)
	}
	call := output.ServerToolCalls[0]
	if call.ToolName != "google_search" || call.ToolType != "google_search" || call.Status != "completed" {
		t.Fatalf("expected completed google_search trace, got %#v", call)
	}
	if call.ArgumentsJSON == "" || call.OutputJSON == "" {
		t.Fatalf("expected google_search input and output details, got %#v", call)
	}
	outputPayload := mustDecodeObject(t, call.OutputJSON)
	if _, ok := outputPayload["groundingChunks"]; ok {
		t.Fatalf("expected compact Gemini search output, got %#v", outputPayload)
	}
	sources := asSlice(outputPayload["sources"])
	if len(sources) != 1 || asMap(sources[0])["url"] != "https://example.com" {
		t.Fatalf("expected compact Gemini sources, got %#v", outputPayload)
	}
}

func TestParseGeminiResponseCapturesURLContextServerToolTrace(t *testing.T) {
	output, err := parseGeminiResponse([]byte(`{
		"candidates": [{
			"content": {"parts": [{"text": "answer"}]},
			"urlContextMetadata": {
				"urlMetadata": [{
					"retrievedUrl": "https://example.com/page",
					"urlRetrievalStatus": "URL_RETRIEVAL_STATUS_SUCCESS"
				}]
			}
		}]
	}`))
	if err != nil {
		t.Fatalf("parse Gemini response: %v", err)
	}
	if output.ServerSideToolUsage["url_context"] != 1 {
		t.Fatalf("expected url_context server-side usage, got %#v", output.ServerSideToolUsage)
	}
	if len(output.ServerToolCalls) != 1 {
		t.Fatalf("expected url_context server-side tool trace, got %#v", output.ServerToolCalls)
	}
	call := output.ServerToolCalls[0]
	if call.ToolName != "url_context" || call.ToolType != "url_context" || call.Status != "completed" {
		t.Fatalf("expected completed url_context trace, got %#v", call)
	}
	if call.ArgumentsJSON == "" || call.OutputJSON == "" {
		t.Fatalf("expected url_context input and output details, got %#v", call)
	}
	outputPayload := mustDecodeObject(t, call.OutputJSON)
	urls := asSlice(outputPayload["urls"])
	if len(urls) != 1 || asMap(urls[0])["url"] != "https://example.com/page" {
		t.Fatalf("expected compact url_context output, got %#v", outputPayload)
	}
}

func TestParseGeminiResponseCapturesCodeExecutionServerToolTrace(t *testing.T) {
	output, err := parseGeminiResponse([]byte(`{
		"candidates": [{
			"content": {
				"parts": [
					{
						"executableCode": {
							"language": "PYTHON",
							"code": "print(1 + 1)"
						}
					},
					{
						"codeExecutionResult": {
							"outcome": "OUTCOME_OK",
							"output": "2\n"
						}
					},
					{"text": "answer"}
				]
			}
		}]
	}`))
	if err != nil {
		t.Fatalf("parse Gemini response: %v", err)
	}
	if output.ServerSideToolUsage["code_execution"] != 1 {
		t.Fatalf("expected code_execution server-side usage, got %#v", output.ServerSideToolUsage)
	}
	if len(output.ServerToolCalls) != 1 {
		t.Fatalf("expected code_execution server-side tool trace, got %#v", output.ServerToolCalls)
	}
	call := output.ServerToolCalls[0]
	if call.ToolName != "code_execution" || call.ToolType != "code_execution" || call.Status != "completed" {
		t.Fatalf("expected completed code_execution trace, got %#v", call)
	}
	if call.ArgumentsJSON == "" || call.OutputJSON == "" || call.ErrorJSON != "" {
		t.Fatalf("expected code_execution input and successful output, got %#v", call)
	}
}

func TestApplyGeminiStreamChunkEmitsServerToolTrace(t *testing.T) {
	result := &portllm.GenerateOutput{ToolCalls: make([]portllm.ToolCall, 0)}
	events := make([]portllm.ToolCall, 0)
	err := applyGeminiStreamChunk(mustDecodeObject(t, `{
		"responseId": "gemini-stream-tool-1",
		"candidates": [{
			"content": {
				"parts": [
					{"executableCode": {"language": "PYTHON", "code": "print(2)"}},
					{"codeExecutionResult": {"outcome": "OUTCOME_OK", "output": "2\n"}}
				]
			}
		}]
	}`), result, func(event portllm.GenerateStreamEvent) error {
		if event.ServerToolCall != nil {
			events = append(events, *event.ServerToolCall)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("apply gemini stream chunk: %v", err)
	}
	if len(result.ServerToolCalls) != 1 || result.ServerToolCalls[0].ToolName != "code_execution" {
		t.Fatalf("expected stored code_execution trace, got %#v", result.ServerToolCalls)
	}
	if len(events) != 1 || events[0].ToolName != "code_execution" || events[0].OutputJSON == "" {
		t.Fatalf("expected emitted code_execution trace, got %#v", events)
	}
}

func TestApplyGeminiStreamChunkEmitsExplicitServerToolInvocation(t *testing.T) {
	result := &portllm.GenerateOutput{ToolCalls: make([]portllm.ToolCall, 0)}
	events := make([]portllm.ToolCall, 0)
	err := applyGeminiStreamChunk(mustDecodeObject(t, `{
		"responseId": "gemini-stream-tool-2",
		"candidates": [{
			"serverSideToolInvocations": [{
				"id": "gsi_1",
				"name": "google_search",
				"status": "running",
				"input": {"query": "SpaceX stock price"}
			}]
		}]
	}`), result, func(event portllm.GenerateStreamEvent) error {
		if event.ServerToolCall != nil {
			events = append(events, *event.ServerToolCall)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("apply gemini stream chunk: %v", err)
	}
	if len(result.ServerToolCalls) != 1 || result.ServerToolCalls[0].Status != "in_progress" {
		t.Fatalf("expected stored in-progress server tool call, got %#v", result.ServerToolCalls)
	}
	if len(events) != 1 || events[0].ToolName != "google_search" || events[0].Status != "in_progress" {
		t.Fatalf("expected emitted google_search invocation, got %#v", events)
	}
}

func TestApplyGeminiStreamChunkKeepsMultipleCodeExecutionTraces(t *testing.T) {
	result := &portllm.GenerateOutput{ToolCalls: make([]portllm.ToolCall, 0)}
	chunks := []string{
		`{"candidates":[{"content":{"parts":[{"executableCode":{"language":"PYTHON","code":"print(1)"}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"codeExecutionResult":{"outcome":"OUTCOME_OK","output":"1\n"}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"executableCode":{"language":"PYTHON","code":"print(2)"}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"codeExecutionResult":{"outcome":"OUTCOME_OK","output":"2\n"}}]}}]}`,
	}
	for _, chunk := range chunks {
		if err := applyGeminiStreamChunk(mustDecodeObject(t, chunk), result, nil); err != nil {
			t.Fatalf("apply gemini stream chunk: %v", err)
		}
	}
	if len(result.ServerToolCalls) != 2 {
		t.Fatalf("expected two code_execution traces, got %#v", result.ServerToolCalls)
	}
	if result.ServerToolCalls[0].ToolCallID == result.ServerToolCalls[1].ToolCallID {
		t.Fatalf("expected distinct code_execution trace ids, got %#v", result.ServerToolCalls)
	}
	if result.ServerToolCalls[0].OutputJSON == "" || result.ServerToolCalls[1].OutputJSON == "" {
		t.Fatalf("expected both code_execution outputs, got %#v", result.ServerToolCalls)
	}
	if result.ServerSideToolUsage["code_execution"] != 2 {
		t.Fatalf("expected cumulative code_execution usage, got %#v", result.ServerSideToolUsage)
	}
}

func TestParseGeminiErrorProvidesUnauthorizedFallback(t *testing.T) {
	err := parseGeminiError(http.StatusUnauthorized, nil, nil)
	var upstreamErr *portllm.UpstreamError
	if !errors.As(err, &upstreamErr) {
		t.Fatalf("expected upstream error, got %v", err)
	}
	if upstreamErr.Message != "google authentication failed; check API key, upstream base URL, and custom auth headers" {
		t.Fatalf("unexpected unauthorized message: %q", upstreamErr.Message)
	}
}
