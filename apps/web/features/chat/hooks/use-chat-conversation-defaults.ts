"use client";

import * as React from "react";

import { listVisibleKnowledgeBases } from "@/shared/api/knowledge-bases";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

export function useChatConversationDefaults({
  conversationID,
  contextKey,
  defaultsPending,
  defaultKnowledgeBaseIDs,
  setSelectedKnowledgeBaseIDs,
}: {
  conversationID: string | null;
  contextKey: string;
  defaultsPending: boolean;
  defaultKnowledgeBaseIDs: string[];
  setSelectedKnowledgeBaseIDs: React.Dispatch<React.SetStateAction<string[]>>;
}) {
  const appliedKnowledgeBaseDefaultsKeyRef = React.useRef("");
  const manuallyChangedKnowledgeBaseKeyRef = React.useRef("");

  React.useEffect(() => {
    if (
      conversationID ||
      defaultsPending ||
      appliedKnowledgeBaseDefaultsKeyRef.current === contextKey ||
      manuallyChangedKnowledgeBaseKeyRef.current === contextKey
    ) {
      return;
    }
    if (defaultKnowledgeBaseIDs.length === 0) {
      appliedKnowledgeBaseDefaultsKeyRef.current = contextKey;
      setSelectedKnowledgeBaseIDs([]);
      return;
    }

    let cancelled = false;
    void (async () => {
      try {
        const token = await resolveAccessToken();
        if (!token) return;
        const available = await listVisibleKnowledgeBases(token, {
          ids: defaultKnowledgeBaseIDs.slice(0, 8),
          pageSize: Math.max(1, Math.min(8, defaultKnowledgeBaseIDs.length)),
        });
        if (cancelled || manuallyChangedKnowledgeBaseKeyRef.current === contextKey) return;
        const readyIDs = new Set(
          available.results.filter((item) => item.readyFileCount > 0).map((item) => item.publicID),
        );
        appliedKnowledgeBaseDefaultsKeyRef.current = contextKey;
        setSelectedKnowledgeBaseIDs(defaultKnowledgeBaseIDs.filter((id) => readyIDs.has(id)).slice(0, 8));
      } catch {
        if (!cancelled && manuallyChangedKnowledgeBaseKeyRef.current !== contextKey) {
          appliedKnowledgeBaseDefaultsKeyRef.current = contextKey;
          setSelectedKnowledgeBaseIDs([]);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [conversationID, contextKey, defaultKnowledgeBaseIDs, defaultsPending, setSelectedKnowledgeBaseIDs]);

  const onSelectedKnowledgeBasesChange = React.useCallback((ids: string[]) => {
    if (!conversationID) {
      manuallyChangedKnowledgeBaseKeyRef.current = contextKey;
    }
    setSelectedKnowledgeBaseIDs(ids.slice(0, 8));
  }, [conversationID, contextKey, setSelectedKnowledgeBaseIDs]);

  return { onSelectedKnowledgeBasesChange };
}
