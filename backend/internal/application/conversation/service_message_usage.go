package conversation

import (
	"context"
	"strings"

	domainchannel "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/channel"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/pkg/traceid"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/tokenestimate"
	"go.uber.org/zap"
)

// messageUsageAccumulator 汇总一条消息内多次 LLM 调用的用量。上报了用量的调用以观测值为准；
// 未上报的调用按请求形状预估输入、按已产出文本预估输出。观测值与预估值按调用互斥，不叠加。
// 输出侧额外记录进行中调用的尾段文本：中断时已完成调用只用观测值（或未上报时的预估），
// 被中断的那次调用取其已上报部分与尾段文本预估的较大者，既不用全文预估覆盖观测值，
// 也不漏掉尚未上报的尾段。
type messageUsageAccumulator struct {
	observedUsage                   llm.Usage
	inputObserved                   bool
	estimatedUnobservedInputTokens  int64
	currentCallEstimatedInputTokens int64

	estimatedUnobservedOutputTokens    int64
	estimatedUnobservedReasoningTokens int64
	// completedObservedUsage 是当前调用开始时的观测快照，与 observedUsage 的差值即当前调用已上报的部分。
	completedObservedUsage   llm.Usage
	currentCallVisibleText   strings.Builder
	currentCallReasoningText strings.Builder
}

func (a *messageUsageAccumulator) beginCall(estimatedInputTokens int64) {
	a.currentCallEstimatedInputTokens = max(estimatedInputTokens, 0)
	a.completedObservedUsage = a.observedUsage
	a.currentCallVisibleText.Reset()
	a.currentCallReasoningText.Reset()
}

// recordCallVisibleText 记录当前调用已产出的可见文本，供上游未上报输出用量时预估。
func (a *messageUsageAccumulator) recordCallVisibleText(text string) {
	a.currentCallVisibleText.WriteString(text)
}

// recordCallReasoningText 记录当前调用已产出的思考文本，供上游未上报输出用量时预估。
func (a *messageUsageAccumulator) recordCallReasoningText(text string) {
	a.currentCallReasoningText.WriteString(text)
}

// finishCall 结束当前调用：上报了对应侧用量则丢弃本次预估，否则把预估计入未观测部分。
func (a *messageUsageAccumulator) finishCall(observedInput bool, observedOutput bool) {
	if observedInput {
		a.markInputObserved()
	} else {
		a.estimatedUnobservedInputTokens += a.currentCallEstimatedInputTokens
		a.currentCallEstimatedInputTokens = 0
	}
	if !observedOutput {
		outputTokens, reasoningTokens := estimateOutputUsage(0, 0, a.currentCallVisibleText.String(), a.currentCallReasoningText.String())
		a.estimatedUnobservedOutputTokens += outputTokens
		a.estimatedUnobservedReasoningTokens += reasoningTokens
	}
	a.currentCallVisibleText.Reset()
	a.currentCallReasoningText.Reset()
}

func (a *messageUsageAccumulator) addObservedUsage(delta llm.Usage) llm.Usage {
	if delta == (llm.Usage{}) {
		return a.observedUsage
	}
	a.observedUsage = addLLMUsage(a.observedUsage, delta)
	if delta.HasObservedInput() {
		a.markInputObserved()
	}
	return a.observedUsage
}

func (a *messageUsageAccumulator) setObservedUsage(usage llm.Usage) {
	a.observedUsage = usage
	if usage.HasObservedInput() {
		a.markInputObserved()
	}
}

func (a *messageUsageAccumulator) markInputObserved() {
	a.inputObserved = true
	a.currentCallEstimatedInputTokens = 0
}

func (a *messageUsageAccumulator) usage() llm.Usage {
	return a.observedUsage
}

func (a *messageUsageAccumulator) interruptedInputTokens() int64 {
	return a.observedUsage.InputTokens + a.estimatedUnobservedInputTokens + a.currentCallEstimatedInputTokens
}

// interruptedOutputTokens 返回中断时计费的输出与思考 token。已完成调用采用观测值加未上报调用的
// 预估；被中断的当前调用取其已上报部分与尾段文本预估的较大者，避免上游按块上报时重复计费。
func (a *messageUsageAccumulator) interruptedOutputTokens() (int64, int64) {
	currentOutputTokens, currentReasoningTokens := estimateOutputUsage(
		max(a.observedUsage.OutputTokens-a.completedObservedUsage.OutputTokens, 0),
		max(a.observedUsage.ReasoningTokens-a.completedObservedUsage.ReasoningTokens, 0),
		a.currentCallVisibleText.String(),
		a.currentCallReasoningText.String(),
	)
	return a.completedObservedUsage.OutputTokens + a.estimatedUnobservedOutputTokens + currentOutputTokens,
		a.completedObservedUsage.ReasoningTokens + a.estimatedUnobservedReasoningTokens + currentReasoningTokens
}

// estimateOutputUsage 用已产出文本补齐一次调用的输出与思考用量：上游拆分上报了思考 token 时两侧
// 分别取较大者；只上报了合并输出时把思考文本并入输出预估，避免重复计费；完全未上报时按文本预估。
func estimateOutputUsage(observedOutputTokens int64, observedReasoningTokens int64, visibleText string, reasoningText string) (int64, int64) {
	estimatedOutputTokens := tokenestimate.Estimate(visibleText)
	estimatedReasoningTokens := tokenestimate.Estimate(reasoningText)
	switch {
	case observedReasoningTokens > 0:
		return resolveObservedOrHigherEstimatedTokens(observedOutputTokens, estimatedOutputTokens),
			resolveObservedOrHigherEstimatedTokens(observedReasoningTokens, estimatedReasoningTokens)
	case observedOutputTokens > 0:
		return resolveObservedOrHigherEstimatedTokens(observedOutputTokens, estimatedOutputTokens+estimatedReasoningTokens), 0
	default:
		return estimatedOutputTokens, estimatedReasoningTokens
	}
}

// effectiveInputTokens 返回本条消息最终计费的非缓存输入。只要有调用上报过输入侧用量，
// 非缓存输入为 0（提示词全部命中缓存）也如实采用；仅在完全没有观测值时才回退到规划预估。
func (a *messageUsageAccumulator) effectiveInputTokens(promptFallback int64) int64 {
	inputTokens := a.observedUsage.InputTokens + a.estimatedUnobservedInputTokens
	if a.inputObserved || inputTokens > 0 {
		return inputTokens
	}
	return max(promptFallback, 0)
}

// effectiveOutputTokens 返回本条消息最终计费的输出与思考 token：观测值加上未上报用量调用的文本预估。
func (a *messageUsageAccumulator) effectiveOutputTokens() (int64, int64) {
	return a.observedUsage.OutputTokens + a.estimatedUnobservedOutputTokens,
		a.observedUsage.ReasoningTokens + a.estimatedUnobservedReasoningTokens
}

func resolveObservedOrHigherEstimatedTokens(observedTokens int64, estimatedTokens int64) int64 {
	if estimatedTokens > observedTokens {
		return estimatedTokens
	}
	if observedTokens > 0 {
		return observedTokens
	}
	return 0
}

// estimateBillableInputTokens 估算一次上游调用实际计费的输入规模。有状态 Responses 续传只发送
// 本轮增量消息，但上游仍按完整上下文计输入，因此估算必须基于完整消息形状而非实际发送的消息。
func estimateBillableInputTokens(input llm.GenerateInput, fullMessages []llm.Message) int64 {
	if strings.TrimSpace(input.PreviousResponseID) == "" {
		return estimateGenerateInputTokens(input)
	}
	return estimateFullShapeInputTokens(input, fullMessages)
}

func estimateGenerateInputTokens(input llm.GenerateInput) int64 {
	tokens := estimatePromptTokens(input.Messages)
	if instructions := strings.TrimSpace(input.Instructions); instructions != "" {
		tokens += tokenestimate.Estimate(instructions) + 4
	}
	if !input.DisableTools {
		tokens += estimateToolDefinitionTokens(input.Tools)
	}
	return tokens
}

func estimateToolDefinitionTokens(tools []llm.ToolDefinition) int64 {
	if len(tools) == 0 {
		return 0
	}
	var tokens int64 = 2
	for _, tool := range tools {
		tokens += tokenestimate.Estimate(tool.Name)
		tokens += tokenestimate.Estimate(tool.Description)
		tokens += tokenestimate.Estimate(string(tool.InputSchema))
		tokens += 12
	}
	return tokens
}

func maxPromptTokenEstimate(values ...int64) int64 {
	var result int64
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}

type promptBudgetFit struct {
	Budget         int64
	TokensBefore   int64
	TokensAfter    int64
	MessagesBefore int
	MessagesAfter  int
	Trimmed        bool
	Exceeded       bool
}

func (s *Service) logPromptBudgetFit(ctx context.Context, modelName string, fit promptBudgetFit) {
	if s == nil || s.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("trace_id", traceid.FromContext(ctx)),
		zap.String("model", strings.TrimSpace(modelName)),
		zap.Int64("effective_budget", fit.Budget),
	}
	if fit.Trimmed {
		s.logger.Info("context_prompt_budget_trimmed", append(fields,
			zap.Int64("tokens_before", fit.TokensBefore),
			zap.Int64("tokens_after", fit.TokensAfter),
			zap.Int("messages_before", fit.MessagesBefore),
			zap.Int("messages_after", fit.MessagesAfter),
		)...)
	}
	if fit.Exceeded {
		s.logger.Warn("context_prompt_required_content_exceeds_budget", append(fields,
			zap.Int64("estimated_tokens", fit.TokensAfter),
		)...)
	}
}

// fitGenerateInputToModelBudget 是初始请求的唯一硬预算入口。它在消息、原生
// instructions 与工具定义全部确定后删除最早的完整历史轮次，保留系统前缀和
// 当前用户轮次。必需内容本身超限时不会破坏当前输入，而是显式返回 Exceeded
// 供调用方记录诊断信息。
func fitGenerateInputToModelBudget(
	input llm.GenerateInput,
	modelName string,
	capabilitiesJSON string,
	fallbackContextWindow int,
	enabled bool,
) (llm.GenerateInput, promptBudgetFit) {
	result := promptBudgetFit{
		MessagesBefore: len(input.Messages),
		MessagesAfter:  len(input.Messages),
		TokensBefore:   estimateGenerateInputTokens(input),
	}
	result.TokensAfter = result.TokensBefore
	if !enabled {
		return input, result
	}

	result.Budget = int64(domainchannel.EffectiveContextBudgetFromCapabilitiesWithFallback(
		modelName,
		capabilitiesJSON,
		fallbackContextWindow,
	))
	if result.TokensBefore <= result.Budget {
		return input, result
	}

	trimmedMessages, trimmed := trimOldestPromptHistory(input.Messages, result.TokensBefore, result.Budget)
	if trimmed {
		input.Messages = trimmedMessages
		result.Trimmed = true
		result.MessagesAfter = len(trimmedMessages)
		result.TokensAfter = estimateGenerateInputTokens(input)
	}
	result.Exceeded = result.TokensAfter > result.Budget
	return input, result
}

// trimOldestPromptHistory 删除最早的完整对话轮次，并始终保留前导系统消息与
// 当前用户轮次。传入的 totalTokens 已包含 tools/instructions 等固定开销。
func trimOldestPromptHistory(messages []llm.Message, totalTokens int64, budget int64) ([]llm.Message, bool) {
	if totalTokens <= budget || len(messages) == 0 {
		return messages, false
	}
	systemEnd, currentUserIndex := toolHistoryBounds(messages)
	if currentUserIndex <= systemEnd {
		return messages, false
	}

	remainingTokens := totalTokens
	for cutFrom := systemEnd; cutFrom < currentUserIndex; cutFrom++ {
		remainingTokens -= estimateMessageTokens(messages[cutFrom])
		nextIndex := cutFrom + 1
		if nextIndex < currentUserIndex && messages[nextIndex].Role != "user" {
			continue
		}
		if remainingTokens <= budget || nextIndex == currentUserIndex {
			trimmed := make([]llm.Message, 0, systemEnd+len(messages)-nextIndex)
			trimmed = append(trimmed, messages[:systemEnd]...)
			trimmed = append(trimmed, messages[nextIndex:]...)
			return trimmed, true
		}
	}
	return messages, false
}

// toolHistoryBounds 定位系统前缀结束位置和当前轮用户消息。
func toolHistoryBounds(messages []llm.Message) (int, int) {
	systemEnd := 0
	for systemEnd < len(messages) && messages[systemEnd].Role == "system" {
		systemEnd++
	}
	currentUserIndex := -1
	for index := len(messages) - 1; index >= systemEnd; index-- {
		if messages[index].Role == "user" {
			currentUserIndex = index
			break
		}
	}
	return systemEnd, currentUserIndex
}

// estimateFullShapeInputTokens 按全量请求形状估算一次上游调用的输入规模。
func estimateFullShapeInputTokens(generateInput llm.GenerateInput, messages []llm.Message) int64 {
	budgetMessages := messages
	if strings.TrimSpace(generateInput.Instructions) != "" {
		_, budgetMessages = extractOpenAIResponsesInstructions(messages)
	}
	budgetInput := generateInput
	budgetInput.Messages = budgetMessages
	budgetInput.PreviousResponseID = ""
	return estimateGenerateInputTokens(budgetInput)
}
