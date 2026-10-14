import type { AdminLLMModelDTO, AdminLLMModelVendor, AdminLLMStatus } from "@/features/admin/api/llm-types";
import { MODEL_KINDS, resolveProtocolLabel } from "@/features/admin/utils/llm-display";
import { parseKindsJSON, stringifyKinds } from "@/entities/model";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

export const PAGE_SIZE_DEFAULT = 25;

export const MODEL_STATUS_OPTIONS: AdminLLMStatus[] = ["active", "inactive"];

export const MODEL_SORT_OPTIONS = [
  { labelKey: "sort.sortOrderAsc", value: "sortOrder_asc" },
  { labelKey: "sort.updatedDesc", value: "updated_desc" },
  { labelKey: "sort.idDesc", value: "id_desc" },
  { labelKey: "sort.platformModelNameAsc", value: "platformModelName_asc" },
  { labelKey: "sort.sourceCountDesc", value: "sourceCount_desc" },
] as const;

export type ModelSortValue = (typeof MODEL_SORT_OPTIONS)[number]["value"];

export const ADAPTER_LABELS: Record<string, string> = {
  openai_responses: resolveProtocolLabel("openai_responses"),
  openrouter_chat_completions: resolveProtocolLabel("openrouter_chat_completions"),
  openrouter_responses: resolveProtocolLabel("openrouter_responses"),
  openai_chat_completions: resolveProtocolLabel("openai_chat_completions"),
  anthropic_messages: resolveProtocolLabel("anthropic_messages"),
  google_generate_content: resolveProtocolLabel("google_generate_content"),
  gemini_interactions: resolveProtocolLabel("gemini_interactions"),
  xai_responses: resolveProtocolLabel("xai_responses"),
};

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Display labels are resolved by consuming components via i18n (kinds.*); only enum values are provided here.
export const MODEL_KIND_OPTIONS = MODEL_KINDS.map((value): { value: string } => ({ value }));

export type ModelFormPayload = {
  platformModelName: string;
  vendor: AdminLLMModelVendor | "";
  kinds: string[];
  icon: string;
  capabilitiesJSON: string;
  systemPrompt: string;
  status: AdminLLMStatus;
  description: string;
};

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

export function formatDateTime(value: string | null | undefined, locale = "en-US"): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function resolveValue(value: string | null | undefined): string {
  return value?.trim() || "-";
}

export function kindsJsonToDisplay(kindsJson: string | null | undefined): string {
  return parseKindsJSON(kindsJson).join(",");
}

export function displayToKindsJson(display: string): string {
  return stringifyKinds(display
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean));
}

export function mapModelToForm(model: AdminLLMModelDTO): ModelFormPayload {
  const kinds = parseKindsJSON(model.kindsJSON);
  return {
    platformModelName: model.platformModelName,
    vendor: model.vendor ?? "",
    kinds,
    icon: model.icon ?? "",
    capabilitiesJSON: model.capabilitiesJSON ?? "",
    systemPrompt: model.systemPrompt ?? "",
    status: model.status,
    description: model.description ?? "",
  };
}
