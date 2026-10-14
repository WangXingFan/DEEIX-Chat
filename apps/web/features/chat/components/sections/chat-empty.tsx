"use client";

import type * as React from "react";
import { AnimatePresence, motion } from "motion/react";

import { cn } from "@/lib/utils";

type ChatEmptyStateProps = {
  greetingTitle: string;
  contentWidthClassName?: string;
  children?: React.ReactNode;
};

const CHAT_EMPTY_TEXT_TRANSITION = {
  duration: 0.22,
  ease: [0.16, 1, 0.3, 1] as const,
};

export function ChatEmptyState({
  greetingTitle,
  contentWidthClassName = "max-w-[1080px]",
  children,
}: ChatEmptyStateProps) {
  return (
    <div className="flex h-full min-h-0 flex-col items-center justify-center px-3 py-12 text-center md:px-6 md:py-20">
      <motion.div
        layout="position"
        transition={CHAT_EMPTY_TEXT_TRANSITION}
        className="relative inline-flex max-w-[calc(100%-4.5rem)] justify-center"
      >
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={greetingTitle}
            className="relative inline-flex min-w-0 items-center justify-center"
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            transition={CHAT_EMPTY_TEXT_TRANSITION}
          >
            <h1 className="min-w-0 text-balance text-[22px] font-medium leading-[1.12] tracking-[-0.005em] text-foreground [font-family:var(--font-economist)] md:text-[32px]">
              {greetingTitle}
            </h1>
          </motion.div>
        </AnimatePresence>
      </motion.div>
      {children ? (
        <motion.div
          layout="position"
          transition={CHAT_EMPTY_TEXT_TRANSITION}
          className={cn("mt-7 w-full md:mt-8", contentWidthClassName)}
        >
          {children}
        </motion.div>
      ) : null}
    </div>
  );
}
