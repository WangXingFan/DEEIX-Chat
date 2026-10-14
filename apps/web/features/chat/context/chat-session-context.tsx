"use client";

import * as React from "react";

import { useChatRunState } from "@/features/chat/hooks/use-chat-run-state";
import type { ConversationRunStore } from "@/features/chat/model/conversation-run-store";

type ChatSessionContextValue = {
  newConversationRevision: number;
  detachConversationRun: (runID: string) => void;
  finishConversationRun: (runID: string) => void;
  registerConversationRun: (runID: string, conversationPublicID: string) => void;
  requestNewConversation: () => void;
};

const ChatSessionContext = React.createContext<ChatSessionContextValue | null>(null);
const ConversationRunStoreContext = React.createContext<ConversationRunStore | null>(null);

export function ChatSessionProvider({ children }: { children: React.ReactNode }) {
  const [newConversationRevision, setNewConversationRevision] = React.useState(0);
  const {
    detachConversationRun,
    finishConversationRun,
    registerConversationRun,
    store,
  } = useChatRunState();
  const requestNewConversation = React.useCallback(() => {
    setNewConversationRevision((prev) => prev + 1);
  }, []);
  const value = React.useMemo(
    () => ({
      newConversationRevision,
      detachConversationRun,
      finishConversationRun,
      registerConversationRun,
      requestNewConversation,
    }),
    [
      detachConversationRun,
      finishConversationRun,
      newConversationRevision,
      registerConversationRun,
      requestNewConversation,
    ],
  );

  return (
    <ConversationRunStoreContext.Provider value={store}>
      <ChatSessionContext.Provider value={value}>{children}</ChatSessionContext.Provider>
    </ConversationRunStoreContext.Provider>
  );
}

export function useChatSession() {
  const context = React.useContext(ChatSessionContext);
  if (!context) {
    throw new Error("useChatSession must be used within ChatSessionProvider");
  }
  return context;
}

export function useConversationRunning(conversationPublicID: string): boolean {
  const store = React.useContext(ConversationRunStoreContext);
  if (!store) {
    throw new Error("useConversationRunning must be used within ChatSessionProvider");
  }
  const conversationID = conversationPublicID.trim();
  const subscribe = React.useCallback(
    (listener: () => void) => store.subscribe(conversationID, listener),
    [conversationID, store],
  );
  const getSnapshot = React.useCallback(
    () => store.isConversationRunning(conversationID),
    [conversationID, store],
  );
  return React.useSyncExternalStore(subscribe, getSnapshot, () => false);
}
