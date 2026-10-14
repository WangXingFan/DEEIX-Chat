"use client";

import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";
import { mergeProcessTraceSnapshot, toPendingProcessTrace } from "@/features/chat/model/message-submit";
import { settleCompletedExchange } from "@/features/chat/model/message-submit-exchange";
import type { PendingAttachment, PendingExchange } from "@/features/chat/types/chat-runtime";
import {
  type ConversationStreamOptions,
  streamMessage as streamConversationMessage,
} from "@/shared/api/conversation";
import type {
  ConversationOptions,
  SendMessageRequest,
  SendMessageResult,
  StreamMessageEvent,
} from "@/shared/api/conversation-types";

// Streaming execution of a single generation: register stream event callbacks, dispatch the request by task type, and settle the exchange on completion.
export function useChatRunStream({
  updatePendingExchange,
  enqueueUpstreamThinkDelta,
  enqueueStreamText,
  flushStreamTextNow,
  flushUpstreamThinkNow,
  resetStreamBuffer,
  setStreamTextSnapshot,
  onConversationRunFinished,
}: {
  updatePendingExchange: (
    exchangeKey: string,
    update: (current: PendingExchange) => PendingExchange,
  ) => void;
  enqueueUpstreamThinkDelta: (
    exchangeKey: string,
    event: Extract<StreamMessageEvent, { type: "upstream_think_delta" }>,
  ) => void;
  enqueueStreamText: (exchangeKey: string, delta: string) => void;
  flushStreamTextNow: (exchangeKey: string) => void;
  flushUpstreamThinkNow: (exchangeKey: string) => void;
  resetStreamBuffer: (exchangeKey?: string) => void;
  setStreamTextSnapshot: (exchangeKey: string, content: string) => void;
  onConversationRunFinished?: (runID: string) => void;
}) {
  const t = useTranslations("chat.submit");

  const runStream = React.useCallback(
    async ({
      token,
      conversationID,
      exchangeKey,
      clientRunID,
      content,
      options,
      effectiveAttachments,
      platformModelName,
      selectedKnowledgeBaseIDs,
      htmlVisualPromptEnabled,
      parentMessagePublicID,
      sourceMessagePublicID,
      branchReason,
      assistantOnlyBranch,
      signal,
    }: {
      token: string;
      conversationID: string;
      exchangeKey: string;
      clientRunID: string;
      content: string;
      options: ConversationOptions;
      effectiveAttachments: PendingAttachment[];
      platformModelName: string;
      selectedKnowledgeBaseIDs: string[];
      htmlVisualPromptEnabled: boolean;
      parentMessagePublicID: string | null;
      sourceMessagePublicID: string | null;
      branchReason: "default" | "retry" | "edit";
      assistantOnlyBranch: boolean;
      signal: AbortSignal;
    }): Promise<SendMessageResult> => {
      let terminalStreamError: Extract<StreamMessageEvent, { type: "error" }> | null = null;
      const streamOptions: ConversationStreamOptions = {
        signal,
        onTerminal: () => {
          onConversationRunFinished?.(clientRunID);
        },
        onInterrupted: (event) => {
          terminalStreamError = event;
        },
        onFileProc: (message) => {
          updatePendingExchange(exchangeKey, (current) => ({
            ...current,
            assistantFileProc: true,
            assistantActivityLabel: message.trim() || t("processingAttachments"),
          }));
        },
        onRagSearch: (message) => {
          updatePendingExchange(exchangeKey, (current) => ({
            ...current,
            assistantFileProc: true,
            assistantActivityLabel: message.trim() || t("retrievingContent"),
          }));
        },
        onCompactDone: (event) => {
          updatePendingExchange(exchangeKey, (current) => ({
            ...current,
            compactDone: {
              method: event.method,
              freed_tokens: event.freed_tokens,
              summary_preview: event.summary_preview,
            },
          }));
        },
        onProcessUpdate: (event) => {
          updatePendingExchange(exchangeKey, (current) => ({
            ...current,
            assistantFileProc: false,
            assistantActivityLabel: undefined,
            assistantProcessTrace: event.trace
              ? mergeProcessTraceSnapshot(current.assistantProcessTrace, toPendingProcessTrace(event.trace))
              : current.assistantProcessTrace,
          }));
        },
        onUpstreamThinkDelta: (event) => {
          enqueueUpstreamThinkDelta(exchangeKey, event);
        },
        onDelta: (delta) => {
          // Always clear assistantFileProc so batched React updates cannot keep the file_proc spinner alive.
          updatePendingExchange(exchangeKey, (current) =>
            current.assistantFileProc
              ? { ...current, assistantFileProc: false, assistantActivityLabel: undefined }
              : current,
          );
          enqueueStreamText(exchangeKey, delta);
        },
        onTextSnapshot: (content) => {
          setStreamTextSnapshot(exchangeKey, content);
        },
        onUsage: (event) => {
          updatePendingExchange(exchangeKey, (current) => ({
            ...current,
            assistantInputTokens: event.input_tokens > 0 ? event.input_tokens : current.assistantInputTokens,
            assistantOutputTokens: event.output_tokens > 0 ? event.output_tokens : current.assistantOutputTokens,
            assistantCacheReadTokens:
              event.cache_read_tokens > 0 ? event.cache_read_tokens : current.assistantCacheReadTokens,
            assistantCacheWriteTokens:
              event.cache_write_tokens > 0 ? event.cache_write_tokens : current.assistantCacheWriteTokens,
            assistantReasoningTokens:
              event.reasoning_tokens > 0 ? event.reasoning_tokens : current.assistantReasoningTokens,
          }));
        },
        onModerationChecking: () => {
          updatePendingExchange(exchangeKey, (current) => ({
            ...current,
            assistantFileProc: true,
            assistantActivityLabel: t("moderationChecking"),
          }));
        },
        onModerationBlocked: (event) => {
          const categories = Array.isArray(event.categories) ? event.categories : [];
          updatePendingExchange(exchangeKey, (current) => ({
            ...current,
            assistantPending: false,
            assistantStreaming: false,
            assistantFileProc: false,
            assistantActivityLabel: undefined,
            assistantText: "",
            assistantAttachments: [],
            assistantProcessTrace: undefined,
            assistantStatus: "blocked",
            assistantErrorCode: "content_moderation.blocked",
            assistantErrorMessage: t("moderationBlocked"),
            assistantInlineAlert: {
              title: t("moderationBlocked"),
              message: [
                t("moderationBlockedDescription"),
                event.eventID ? t("moderationEventId", { id: event.eventID }) : "",
                categories.length > 0 ? t("moderationCategories", { categories: categories.join(", ") }) : "",
                event.billedReason ? t("moderationBlockedBilled") : "",
              ]
                .filter(Boolean)
                .join("\n"),
            },
          }));
          toast.error(t("moderationBlocked"), {
            description: t("moderationBlockedDescription"),
          });
        },
      };

      const chatPayload: SendMessageRequest = {
        model: platformModelName,
        options: Object.keys(options).length > 0 ? options : undefined,
        clientRunID,
        fileIDs: effectiveAttachments.length > 0 ? effectiveAttachments.map((item) => item.fileID) : undefined,
        parentMessagePublicID: parentMessagePublicID || undefined,
        sourceMessagePublicID: sourceMessagePublicID || undefined,
        branchReason,
        contentType: effectiveAttachments.length > 0 ? "mixed" : "text",
        content,
        knowledgeBaseIDs: selectedKnowledgeBaseIDs,
        htmlVisualPrompt: htmlVisualPromptEnabled || undefined,
      };
      const completed = await streamConversationMessage(token, conversationID, chatPayload, streamOptions);

      flushStreamTextNow(exchangeKey);
      flushUpstreamThinkNow(exchangeKey);
      resetStreamBuffer(exchangeKey);
      updatePendingExchange(exchangeKey, (current) =>
        settleCompletedExchange(current, completed, {
          assistantOnlyBranch,
          clientRunID,
          terminalStreamError,
          t,
        }),
      );
      return completed;
    },
    [
      enqueueStreamText,
      enqueueUpstreamThinkDelta,
      flushStreamTextNow,
      flushUpstreamThinkNow,
      onConversationRunFinished,
      resetStreamBuffer,
      setStreamTextSnapshot,
      t,
      updatePendingExchange,
    ],
  );

  return { runStream };
}
