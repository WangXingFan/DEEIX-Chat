"use client";

import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";
import type { PendingAttachment } from "@/features/chat/types/chat-runtime";
import type { FileObjectDTO } from "@/shared/api/file-types";

/**
 * Attaches an existing file from the file center to the composer, keeping the per-message count limit.
 */
export function useChatAttachmentActions({
  attachments,
  maxFilesPerMessage,
  setAttachments,
}: {
  attachments: PendingAttachment[];
  maxFilesPerMessage: number;
  setAttachments: React.Dispatch<React.SetStateAction<PendingAttachment[]>>;
}) {
  const t = useTranslations("chat");

  const onAttachExistingFile = React.useCallback(
    (file: FileObjectDTO) => {
      const alreadyAttached = attachments.some((item) => item.fileID === file.fileID);
      if (alreadyAttached) {
        return;
      }
      if (maxFilesPerMessage > 0 && attachments.length >= maxFilesPerMessage) {
        toast.error(t("attachments.limitReached"), {
          description: t("attachments.maxUploadFiles", { count: maxFilesPerMessage }),
        });
        return;
      }
      setAttachments((previous) => {
        if (previous.some((item) => item.fileID === file.fileID)) {
          return previous;
        }
        return [
          ...previous,
          {
            fileID: file.fileID,
            fileName: file.fileName,
            mimeType: file.mimeType,
            detectedMime: file.detectedMIME,
            fileCategory: file.fileCategory,
            sizeBytes: file.sizeBytes,
            processingStatus: file.processingStatus,
            processingReady: file.processingReady,
            processingErrorCode: file.processingErrorCode,
            processingErrorMessage: file.processingErrorMessage,
            extractStatus: file.extractStatus,
            embedStatus: file.embedStatus,
            ragReady: false,
            ragReason: "",
            ocrUsed: false,
            ragOptOut: file.ragOptOut,
          },
        ];
      });
    },
    [attachments, maxFilesPerMessage, setAttachments, t],
  );

  return {
    onAttachExistingFile,
  };
}
