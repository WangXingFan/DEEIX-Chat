"use client";

import * as React from "react";

import { getAdminReferenceData, listAdminSettingsByNamespace } from "@/features/admin/api";
import type { AdminLLMModelDTO } from "@/features/admin/api/llm-types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

type UseAdminModelsCapabilityContextOptions = {
  open: boolean;
  models: AdminLLMModelDTO[];
};

// Read-only context the model sheet's capability editor needs: the full model
// list for presets and the global context-window fallback. Every failure keeps
// the previous/default value on purpose.
export function useAdminModelsCapabilityContext({ open, models }: UseAdminModelsCapabilityContextOptions) {
  const [capabilitySourceModels, setCapabilitySourceModels] = React.useState<AdminLLMModelDTO[]>(models);
  const [contextWindowFallbackTokens, setContextWindowFallbackTokens] = React.useState(128_000);

  React.useEffect(() => {
    if (!open) {
      setCapabilitySourceModels(models);
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const token = await resolveAccessToken();
        if (!token) {
          return;
        }
        const referenceData = await getAdminReferenceData(token);
        if (!cancelled) {
          setCapabilitySourceModels(referenceData.models);
        }
      } catch {
        if (!cancelled) {
          setCapabilitySourceModels(models);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [models, open]);

  React.useEffect(() => {
    if (!open) {
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const token = await resolveAccessToken();
        if (!token) return;
        const settings = await listAdminSettingsByNamespace(token, "chat");
        const rawValue = settings.find((item) => item.key === "context_window_fallback_tokens")?.value;
        const parsedValue = Number(rawValue);
        if (!cancelled && Number.isSafeInteger(parsedValue) && parsedValue >= 4_096 && parsedValue <= 16_000_000) {
          setContextWindowFallbackTokens(parsedValue);
        }
      } catch {
        // Keep system defaults if settings fail to load; existing models still prefer the effective window returned by the backend.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open]);

  return { capabilitySourceModels, contextWindowFallbackTokens };
}
