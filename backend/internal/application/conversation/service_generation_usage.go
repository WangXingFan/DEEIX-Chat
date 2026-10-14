package conversation

import (
	"strings"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

// syncUpstreamOutputThinking 把最终上游输出中的结构化思考同步进轨迹，并返回剥离思考段后的正文。
func syncUpstreamOutputThinking(traceRecorder *messageTraceRecorder, output *llm.GenerateOutput) string {
	if output == nil {
		return ""
	}
	assistantText, extractedThink := splitAssistantOutputThinkingContent(output.Text)
	if assistantText == "" && strings.TrimSpace(extractedThink) == "" {
		assistantText = strings.TrimSpace(output.Text)
	}
	if traceRecorder != nil && output.Reasoning != nil {
		traceRecorder.syncStructuredThink(
			output.Reasoning.Text,
			output.Reasoning.Summary,
			reasoningPayload(&llm.ReasoningDelta{
				EventType:        "response.completed",
				ItemID:           output.Reasoning.ItemID,
				Status:           output.Reasoning.Status,
				Kind:             messageTraceThinkKindContent,
				EncryptedContent: output.Reasoning.EncryptedContent,
			}),
		)
	} else if traceRecorder != nil && strings.TrimSpace(extractedThink) != "" {
		traceRecorder.syncStructuredThink(extractedThink, "", nil)
	}
	if traceRecorder != nil {
		traceRecorder.completeUpstreamThink()
	}
	return assistantText
}

// finalizeStreamingOutputThinking 对最终推理快照进行对账，且不重放流中已发出的 trace 事件。
func finalizeStreamingOutputThinking(traceRecorder *messageTraceRecorder, output *llm.GenerateOutput) {
	if traceRecorder == nil || output == nil {
		return
	}
	if output.Reasoning != nil {
		traceRecorder.reconcileStructuredThink(
			output.Reasoning.Text,
			output.Reasoning.Summary,
			reasoningPayload(&llm.ReasoningDelta{
				EventType:        "response.completed",
				ItemID:           output.Reasoning.ItemID,
				Status:           output.Reasoning.Status,
				Kind:             messageTraceThinkKindContent,
				EncryptedContent: output.Reasoning.EncryptedContent,
			}),
		)
	}
	traceRecorder.completeUpstreamThink()
}

func outputReasoningContent(output *llm.GenerateOutput) string {
	if output == nil {
		return ""
	}
	if output.Reasoning != nil {
		if text := strings.TrimSpace(output.Reasoning.Text); text != "" {
			return text
		}
	}
	_, extractedThink := splitAssistantOutputThinkingContent(output.Text)
	return strings.TrimSpace(extractedThink)
}

func addLLMUsage(left llm.Usage, right llm.Usage) llm.Usage {
	return llm.Usage{
		InputTokens:        left.InputTokens + right.InputTokens,
		OutputTokens:       left.OutputTokens + right.OutputTokens,
		CacheReadTokens:    left.CacheReadTokens + right.CacheReadTokens,
		CacheWriteTokens:   left.CacheWriteTokens + right.CacheWriteTokens,
		CacheWrite5mTokens: left.CacheWrite5mTokens + right.CacheWrite5mTokens,
		CacheWrite1hTokens: left.CacheWrite1hTokens + right.CacheWrite1hTokens,
		ReasoningTokens:    left.ReasoningTokens + right.ReasoningTokens,
		Speed:              mergeLLMUsageSpeed(left.Speed, right.Speed),
		ServiceTier:        mergeLLMUsageServiceTier(left.ServiceTier, right.ServiceTier),
		RawUsageJSON:       llm.MergeRawUsageJSON(left.RawUsageJSON, right.RawUsageJSON),
	}
}

func diffLLMUsage(current llm.Usage, previous llm.Usage) llm.Usage {
	return llm.Usage{
		InputTokens:        nonNegativeTokenDelta(current.InputTokens, previous.InputTokens),
		OutputTokens:       nonNegativeTokenDelta(current.OutputTokens, previous.OutputTokens),
		CacheReadTokens:    nonNegativeTokenDelta(current.CacheReadTokens, previous.CacheReadTokens),
		CacheWriteTokens:   nonNegativeTokenDelta(current.CacheWriteTokens, previous.CacheWriteTokens),
		CacheWrite5mTokens: nonNegativeTokenDelta(current.CacheWrite5mTokens, previous.CacheWrite5mTokens),
		CacheWrite1hTokens: nonNegativeTokenDelta(current.CacheWrite1hTokens, previous.CacheWrite1hTokens),
		ReasoningTokens:    nonNegativeTokenDelta(current.ReasoningTokens, previous.ReasoningTokens),
		Speed:              strings.TrimSpace(current.Speed),
		ServiceTier:        strings.TrimSpace(current.ServiceTier),
		RawUsageJSON:       diffLLMUsageRawJSON(current.RawUsageJSON, previous.RawUsageJSON),
	}
}

func nonNegativeTokenDelta(current int64, previous int64) int64 {
	if current <= previous {
		return 0
	}
	return current - previous
}

func emitLLMUsageEvent(onEvent func(eventType string, payload map[string]any) error, usage llm.Usage) error {
	if onEvent == nil || usage == (llm.Usage{}) {
		return nil
	}
	return onEvent("usage", map[string]any{
		"input_tokens":       usage.InputTokens,
		"output_tokens":      usage.OutputTokens,
		"cache_read_tokens":  usage.CacheReadTokens,
		"cache_write_tokens": usage.CacheWriteTokens,
		"reasoning_tokens":   usage.ReasoningTokens,
	})
}

func diffLLMUsageRawJSON(current string, previous string) string {
	current = strings.TrimSpace(current)
	if current == "" || current == strings.TrimSpace(previous) {
		return ""
	}
	return current
}

func mergeLLMUsageSpeed(left string, right string) string {
	left = strings.TrimSpace(strings.ToLower(left))
	right = strings.TrimSpace(strings.ToLower(right))
	if left == "fast" || right == "fast" {
		return "fast"
	}
	if right != "" {
		return right
	}
	return left
}

func mergeLLMUsageServiceTier(left string, right string) string {
	left = strings.TrimSpace(strings.ToLower(left))
	right = strings.TrimSpace(strings.ToLower(right))
	if right != "" {
		return right
	}
	return left
}
