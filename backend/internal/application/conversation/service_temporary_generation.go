package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
)

// temporaryGenerationResult 汇总一次临时对话上游调用的输出、用量与首字延迟。
type temporaryGenerationResult struct {
	Output            *llm.GenerateOutput
	Usage             llm.Usage
	FirstTokenLatency int64
	// LLMCallCount 是成功返回的上游 LLM 调用数，供按次计费结算。
	LLMCallCount int
}

type temporaryGenerationInput struct {
	Request       TemporaryChatInput
	Route         *channel.ResolvedRoute
	RouteConfig   llm.RouteConfig
	InitialInput  llm.GenerateInput
	TraceRecorder *messageTraceRecorder
	StartedAt     time.Time
	OnDelta       func(string) error
}

// runTemporaryGeneration 执行临时对话的上游调用。临时对话不注册工具，
// 每次调用都发送完整上下文，并始终受临时隐私边界约束。
func (s *Service) runTemporaryGeneration(ctx context.Context, input temporaryGenerationInput) (temporaryGenerationResult, error) {
	request := input.Request
	route := input.Route
	routeConfig := input.RouteConfig
	totalUsage := llm.Usage{}
	firstTokenLatency := int64(0)
	completedLLMCallCount := 0

	currentInput := enforceTemporaryGenerateInput(input.InitialInput)
	var callText strings.Builder
	var observedUsage llm.Usage
	output, err := s.llmClient.GenerateStream(ctx, routeConfig, currentInput, func(event llm.GenerateStreamEvent) error {
		if event.Usage != (llm.Usage{}) {
			observedUsage = event.Usage
			if emitErr := emitLLMUsageEvent(request.OnEvent, addLLMUsage(totalUsage, observedUsage)); emitErr != nil {
				return emitErr
			}
		}
		if input.TraceRecorder != nil && event.Reasoning != nil && event.Reasoning.Text != "" {
			input.TraceRecorder.appendUpstreamReasoning(event.Reasoning.Kind, event.Reasoning.Text, reasoningPayload(event.Reasoning))
			if strings.EqualFold(strings.TrimSpace(event.Reasoning.Status), "completed") {
				input.TraceRecorder.completeUpstreamThink()
			}
		}
		if event.Delta == "" {
			return nil
		}
		if firstTokenLatency == 0 {
			firstTokenLatency = time.Since(input.StartedAt).Milliseconds()
		}
		if input.TraceRecorder != nil {
			input.TraceRecorder.completeProcess()
			input.TraceRecorder.completeUpstreamThink()
		}
		callText.WriteString(event.Delta)
		if input.OnDelta != nil {
			return input.OnDelta(event.Delta)
		}
		return nil
	})
	if output == nil {
		output = &llm.GenerateOutput{}
	}
	if output.Text == "" {
		output.Text = callText.String()
	}
	if output.Usage == (llm.Usage{}) {
		output.Usage = observedUsage
	}
	totalUsage = addLLMUsage(totalUsage, output.Usage)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			s.routeResolver.MarkRouteFailure(ctx, route, err)
		}
		return temporaryGenerationResult{Output: output, Usage: totalUsage, FirstTokenLatency: firstTokenLatency, LLMCallCount: completedLLMCallCount}, err
	}
	s.routeResolver.MarkRouteSuccess(ctx, route)
	completedLLMCallCount++
	return temporaryGenerationResult{Output: output, Usage: totalUsage, FirstTokenLatency: firstTokenLatency, LLMCallCount: completedLLMCallCount}, nil
}
