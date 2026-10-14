"use client";

import { useTranslations } from "next-intl";
import * as React from "react";
import { sanitizeConversationOptions } from "@/features/chat/model/conversation-options";
import {
  type ChatModelOption,
  MODEL_OPTION_CONTROL_TYPES,
  type ModelOptionControl,
  type ModelOptionControlType,
} from "@/features/chat/types/chat-runtime";
import { parseSendShortcut, type SendShortcut } from "@/features/settings";
import { getBillingConfig } from "@/shared/api/billing";
import { listConversationRuns } from "@/shared/api/conversation";
import type { ConversationOptions } from "@/shared/api/conversation-types";
import { listPublicModels, MODEL_CATALOG_CHANGED_EVENT } from "@/shared/api/model";
import type { PublicModelDTO } from "@/shared/api/model-types";
import { getModelOptionPolicy } from "@/shared/api/settings";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { type BillingDisplayCurrency, normalizeBillingDisplayCurrency } from "@/entities/billing";
import {
  type ModelOptionPolicy,
  parseKindsJSON,
  parseProtocolsJSON,
} from "@/entities/model";
import { type ChatContentWidth, parseChatContentWidth, useUserSettings } from "@/entities/user-settings";
import { resolveConversationDefaultModel } from "@/entities/conversation";
import { isOneOf, isRecord, parseJSON } from "@/shared/lib/type-guards";

type ModelCatalogRefreshResult = {
  models: PublicModelDTO[];
  modelOptionPolicy: ModelOptionPolicy | null;
};

function parseJSONObject(raw: string): Record<string, unknown> | null {
  const normalized = raw.trim();
  if (!normalized) {
    return null;
  }
  const parsed = parseJSON(normalized);
  return isRecord(parsed) ? parsed : null;
}

function resolveChatContentWidth(settings: Record<string, string>): ChatContentWidth {
  return parseChatContentWidth(settings["chat.content_width"]);
}

function resolveDefaultOptions(raw: string): ConversationOptions {
  const parsed = parseJSONObject(raw);
  if (!parsed) {
    return {};
  }
  const defaults = parsed.defaultOptions;
  const defaultOptions = isRecord(defaults) ? sanitizeConversationOptions(defaults) : {};
  return defaultOptions;
}

const isModelOptionControlType = isOneOf(MODEL_OPTION_CONTROL_TYPES);

function normalizeOptionControlPath(value: unknown): string {
  if (typeof value !== "string") {
    return "";
  }
  return value
    .split(".")
    .map((segment) => segment.trim())
    .filter(Boolean)
    .join(".");
}

function normalizeOptionControlType(value: unknown): ModelOptionControlType | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const normalized = value.trim();
  return isModelOptionControlType(normalized) ? normalized : undefined;
}

function normalizeOptionControlString(value: unknown): string | undefined {
  if (typeof value !== "string") {
    return undefined;
  }
  const normalized = value.trim();
  return normalized || undefined;
}

function normalizeOptionControlOptions(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const options = Array.from(
    new Set(
      value
        .map((item) => (typeof item === "string" ? item.trim() : ""))
        .filter(Boolean),
    ),
  );
  return options.length > 0 ? options : undefined;
}

function resolveLockedOptionPaths(raw: string): string[] {
  const parsed = parseJSONObject(raw);
  const rawPaths = parsed?.lockedOptionPaths;
  if (!Array.isArray(rawPaths)) {
    return [];
  }
  return Array.from(
    new Set(
      rawPaths
        .map((item) => normalizeOptionControlPath(item))
        .filter(Boolean),
    ),
  );
}

function resolveOptionControls(raw: string): ModelOptionControl[] {
  const parsed = parseJSONObject(raw);
  const rawControls = parsed?.optionControls;
  if (!Array.isArray(rawControls)) {
    return [];
  }
  const lockedPaths = new Set(resolveLockedOptionPaths(raw));

  const controls = rawControls.flatMap((source: unknown): ModelOptionControl[] => {
    if (!isRecord(source)) {
      return [];
    }
    const path = normalizeOptionControlPath(source.path);
    if (!path) {
      return [];
    }
    const control: ModelOptionControl = { path };
    if (lockedPaths.has(path)) {
      control.locked = true;
    }
    const type = normalizeOptionControlType(source.type);
    const label = normalizeOptionControlString(source.label);
    const description = normalizeOptionControlString(source.description);
    const placeholder = normalizeOptionControlString(source.placeholder);
    const options = normalizeOptionControlOptions(source.options);
    if (type) {
      control.type = type;
    }
    if (label) {
      control.label = label;
    }
    if (description) {
      control.description = description;
    }
    if (placeholder) {
      control.placeholder = placeholder;
    }
    if (options) {
      control.options = options;
    }
    return [control];
  });

  return controls.filter((item, index) => controls.findIndex((candidate) => candidate.path === item.path) === index);
}

function toChatModelOption(item: PublicModelDTO): ChatModelOption {
  const protocols = parseProtocolsJSON(item.protocolsJSON);
  return {
    platformModelName: item.platformModelName,
    displayName: item.displayName,
    icon: item.icon,
    vendor: item.vendor,
    vendorName: item.vendorName,
    vendorIcon: item.vendorIcon,
    displayGroupID: item.displayGroupID,
    displayGroupName: item.displayGroupName,
    displayGroupIcon: item.displayGroupIcon,
    kinds: parseKindsJSON(item.kindsJSON),
    protocols,
    defaultOptions: resolveDefaultOptions(item.capabilitiesJSON),
    optionControls: resolveOptionControls(item.capabilitiesJSON),
    lockedOptionPaths: resolveLockedOptionPaths(item.capabilitiesJSON),
    pricing: item.pricing,
  };
}

export function useChatModelOptions({
  conversationPublicID,
  conversationModel,
  resetToken,
}: {
  conversationPublicID: string | null;
  conversationModel?: string | null;
  resetToken?: number;
}) {
  const t = useTranslations("chat.models");
  const { settings: userSettings } = useUserSettings();
  const [availableModels, setAvailableModels] = React.useState<PublicModelDTO[]>([]);
  const [modelsLoading, setModelsLoading] = React.useState(true);
  const [defaultModelResolving, setDefaultModelResolving] = React.useState(false);
  const [modelsErrorMsg, setModelsErrorMsg] = React.useState("");
  const [selectedPlatformModelName, setSelectedPlatformModelName] = React.useState("");
  const [billingCostAvailable, setBillingCostAvailable] = React.useState(false);
  const [billingDisplayCurrency, setBillingDisplayCurrency] = React.useState<BillingDisplayCurrency>("USD");
  const [billingDisplayUsdToCnyRate, setBillingDisplayUsdToCnyRate] = React.useState<number | null>(null);
  const [modelOptionPolicy, setModelOptionPolicy] = React.useState<ModelOptionPolicy | null>(null);
  const activeConversationRef = React.useRef<string | null>(null);
  const userSelectedModelRef = React.useRef(false);
  const previousResetTokenRef = React.useRef(resetToken);
  const runModelRequestRef = React.useRef(0);
  const modelCatalogRequestRef = React.useRef<Promise<ModelCatalogRefreshResult> | null>(null);
  const userDefaultModel = userSettings["chat.default_model"]?.trim() ?? "";
  const sendShortcut: SendShortcut = parseSendShortcut(userSettings["chat.send_on_enter"]);
  const restoreDraftOnFailure = userSettings["chat.restore_draft_on_failure"] !== "false";
  const preserveConversationDrafts = userSettings["chat.preserve_conversation_drafts"] !== "false";
  const inputHeight: "compact" | "standard" | "loose" =
    userSettings["chat.input_height"] === "compact" || userSettings["chat.input_height"] === "loose"
      ? userSettings["chat.input_height"]
      : "standard";
  const contentWidth: ChatContentWidth = resolveChatContentWidth(userSettings);
  const markdownRender = userSettings["chat.markdown_render"] !== "false";
  const showModelInfo = userSettings["chat.show_model_info"] !== "false";
  const showLatency = userSettings["chat.show_latency"] !== "false";
  const showTokenUsage = userSettings["chat.show_token_usage"] !== "false";
  const showBillingCost = billingCostAvailable && userSettings["chat.show_billing_cost"] !== "false";
  const showProcessTrace = userSettings["chat.show_process_trace"] !== "false";

  const selectPlatformModelName = React.useCallback((platformModelName: string) => {
    userSelectedModelRef.current = true;
    setSelectedPlatformModelName(platformModelName);
  }, []);

  const loadModelCatalog = React.useCallback((accessToken?: string): Promise<ModelCatalogRefreshResult> => {
    if (modelCatalogRequestRef.current) {
      return modelCatalogRequestRef.current;
    }

    let request: Promise<ModelCatalogRefreshResult>;
    request = (async () => {
      const token = accessToken?.trim() || await resolveAccessToken();
      if (!token) {
        throw new Error("missing access token");
      }

      const [models, modelOptionPolicy] = await Promise.all([
        listPublicModels(token),
        getModelOptionPolicy(token).catch((): null => null),
      ]);
      return { models, modelOptionPolicy };
    })().finally(() => {
      if (modelCatalogRequestRef.current === request) {
        modelCatalogRequestRef.current = null;
      }
    });

    modelCatalogRequestRef.current = request;
    return request;
  }, []);

  const applyModelCatalog = React.useCallback((catalog: ModelCatalogRefreshResult) => {
    setAvailableModels(catalog.models);
    setModelOptionPolicy(catalog.modelOptionPolicy);
  }, []);

  const refreshModelCatalog = React.useCallback(async (): Promise<ModelCatalogRefreshResult> => {
    const catalog = await loadModelCatalog();
    applyModelCatalog(catalog);
    setModelsErrorMsg("");
    return catalog;
  }, [applyModelCatalog, loadModelCatalog]);

  React.useEffect(() => {
    const handleCatalogChanged = () => {
      void refreshModelCatalog().catch(() => setModelsErrorMsg(t("loadFailed")));
    };
    window.addEventListener(MODEL_CATALOG_CHANGED_EVENT, handleCatalogChanged);
    return () => window.removeEventListener(MODEL_CATALOG_CHANGED_EVENT, handleCatalogChanged);
  }, [refreshModelCatalog, t]);

  const refreshModelOption = React.useCallback(async (platformModelName: string): Promise<ChatModelOption | null> => {
    const normalizedName = platformModelName.trim();
    if (!normalizedName) {
      return null;
    }

    const catalog = await refreshModelCatalog();
    const nextModel = catalog.models.find((item) => item.platformModelName === normalizedName);
    return nextModel ? toChatModelOption(nextModel) : null;
  }, [refreshModelCatalog]);

  React.useEffect(() => {
    let cancelled = false;

    async function loadModels() {
      setModelsLoading(true);
      setModelsErrorMsg("");
      try {
        const token = await resolveAccessToken();
        if (!token) {
          setModelsErrorMsg(t("signInRequired"));
          return;
        }
        const [catalog, billingConfig] = await Promise.all([
          loadModelCatalog(token),
          getBillingConfig(token).catch((): null => null),
        ]);
        if (cancelled) {
          return;
        }
        applyModelCatalog(catalog);
        setBillingCostAvailable((billingConfig?.config.mode ?? "self") !== "self");
        setBillingDisplayCurrency(normalizeBillingDisplayCurrency(billingConfig?.config.displayCurrency));
        setBillingDisplayUsdToCnyRate(billingConfig?.config.usdToCNYRate ?? null);
      } catch {
        if (!cancelled) {
          setModelsErrorMsg(t("loadFailed"));
        }
      } finally {
        if (!cancelled) {
          setModelsLoading(false);
        }
      }
    }

    void loadModels();
    return () => {
      cancelled = true;
    };
  }, [applyModelCatalog, loadModelCatalog, t]);

  React.useEffect(() => {
    if (previousResetTokenRef.current === resetToken) {
      return;
    }
    previousResetTokenRef.current = resetToken;
    userSelectedModelRef.current = false;
  }, [resetToken]);

  React.useEffect(() => {
    const normalizedConversationID = conversationPublicID?.trim() || null;
    if (!normalizedConversationID) {
      // Ordinary re-renders without a conversation keep the manual selection; an explicit new chat resets via resetToken.
      activeConversationRef.current = null;
      return;
    }

    const conversationChanged = activeConversationRef.current !== normalizedConversationID;
    if (conversationChanged) {
      activeConversationRef.current = normalizedConversationID;
      userSelectedModelRef.current = false;
    }

    const fallbackModel = conversationModel?.trim() || "";
    if (!userSelectedModelRef.current) {
      setSelectedPlatformModelName(fallbackModel);
    }

    let cancelled = false;
    const requestID = runModelRequestRef.current + 1;
    runModelRequestRef.current = requestID;

    // Conversation ID bound to this request (non-empty).
    const activeConversationID = normalizedConversationID;

    async function loadLatestRunModel() {
      const token = await resolveAccessToken();
      if (!token) {
        return;
      }

      const runs = await listConversationRuns(token, activeConversationID, { page: 1, pageSize: 1 });
      if (cancelled || requestID !== runModelRequestRef.current || userSelectedModelRef.current) {
        return;
      }

      const latestRunModel = runs.results[0]?.platformModelName?.trim() || "";
      setSelectedPlatformModelName(latestRunModel || fallbackModel);
    }

    void loadLatestRunModel().catch((): undefined => undefined);

    return () => {
      cancelled = true;
    };
  }, [conversationModel, conversationPublicID, resetToken]);

  React.useEffect(() => {
    if (availableModels.length === 0) {
      setDefaultModelResolving(false);
      return;
    }
    if (conversationPublicID?.trim()) {
      setDefaultModelResolving(false);
      return;
    }

    let cancelled = false;
    setDefaultModelResolving(true);
    async function applyDefaultModel() {
      const token = await resolveAccessToken();
      if (!token || cancelled || userSelectedModelRef.current) {
        return;
      }
      const result = await resolveConversationDefaultModel({
        accessToken: token,
        availableModels,
        userDefaultModel,
      });
      if (!cancelled && !userSelectedModelRef.current) {
        setSelectedPlatformModelName(result.platformModelName);
      }
    }

    void applyDefaultModel().catch(() => {
      if (!cancelled && !userSelectedModelRef.current) {
        setSelectedPlatformModelName(availableModels[0]?.platformModelName ?? "");
      }
    }).finally(() => {
      if (!cancelled) {
        setDefaultModelResolving(false);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [availableModels, conversationPublicID, resetToken, userDefaultModel]);

  const modelOptions = React.useMemo<ChatModelOption[]>(
    () =>
      availableModels.map((model) => toChatModelOption(model)),
    [availableModels],
  );

  return {
    modelOptions,
    refreshModelCatalog,
    refreshModelOption,
    modelsLoading: modelsLoading || defaultModelResolving,
    modelsErrorMsg,
    sendShortcut,
    restoreDraftOnFailure,
    preserveConversationDrafts,
    inputHeight,
    contentWidth,
    markdownRender,
    showModelInfo,
    showLatency,
    showTokenUsage,
    showBillingCost,
    showProcessTrace,
    billingDisplayCurrency,
    billingDisplayUsdToCnyRate,
    modelOptionPolicy,
    selectedPlatformModelName,
    setSelectedPlatformModelName: selectPlatformModelName,
  };
}
