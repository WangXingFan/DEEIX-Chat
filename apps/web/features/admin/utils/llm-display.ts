import type { AdminLLMAdapter } from "@/features/admin/api/llm-types";
import { UPSTREAM_COMPATIBLE_OPTIONS } from "@/entities/model";

// Model kind enum; display labels always go through i18n (kinds.* in the adminModels/adminUpstreams namespaces), so no English labels are kept here.
export const MODEL_KINDS = ["chat", "audio"] as const;

// Brand names are proper nouns and are not translated; "custom" is resolved by callers via i18n (compatible.custom).
export const COMPATIBLE_OPTIONS = UPSTREAM_COMPATIBLE_OPTIONS;

type ProtocolOption = {
  value: AdminLLMAdapter;
  label: string;
  kinds: readonly string[];
};

// Protocol display order: vendors OpenAI → Anthropic → Google → xAI → OpenRouter;
// within a vendor, Chat Completions → Responses.
// This order applies to all sorted protocol displays via PROTOCOL_DISPLAY_ORDER.
export const PROTOCOL_OPTIONS: ReadonlyArray<ProtocolOption> = [
  { value: "openai_chat_completions", label: "Chat Completions (OpenAI)", kinds: ["chat"] },
  { value: "openai_responses", label: "Responses (OpenAI)", kinds: ["chat"] },
  { value: "anthropic_messages", label: "Messages (Anthropic)", kinds: ["chat"] },
  { value: "google_generate_content", label: "Generate Content (Google)", kinds: ["chat"] },
  { value: "gemini_interactions", label: "Interactions (Google)", kinds: ["chat"] },
  { value: "xai_responses", label: "Responses (xAI)", kinds: ["chat"] },
  { value: "openrouter_chat_completions", label: "Chat Completions (OpenRouter)", kinds: ["chat"] },
  { value: "openrouter_responses", label: "Responses (OpenRouter)", kinds: ["chat"] },
] as const;

const PROTOCOL_LABELS: Record<string, string> = {
  ...Object.fromEntries(PROTOCOL_OPTIONS.map((item) => [item.value, item.label])),
};

const PROTOCOL_KINDS: Record<string, readonly string[]> = {
  ...Object.fromEntries(PROTOCOL_OPTIONS.map((item) => [item.value, item.kinds])),
};

const PROTOCOL_DISPLAY_ORDER = new Map<string, number>(
  PROTOCOL_OPTIONS.map((item, index) => [item.value, index]),
);

// Protocol names are technical terms (matching upstream API endpoints), shown in English and not translated.
export function resolveProtocolLabel(protocol: string): string {
  return PROTOCOL_LABELS[protocol] ?? protocol;
}

export function sortProtocolsForDisplay<T extends string>(protocols: readonly T[]): T[] {
  const seen = new Set<string>();
  return protocols
    .map((protocol, index) => ({ protocol, index }))
    .filter(({ protocol }) => {
      const key = String(protocol || "").trim();
      if (!key || seen.has(key)) {
        return false;
      }
      seen.add(key);
      return true;
    })
    .sort((a, b) => {
      const orderA = PROTOCOL_DISPLAY_ORDER.get(a.protocol);
      const orderB = PROTOCOL_DISPLAY_ORDER.get(b.protocol);
      if (orderA !== undefined && orderB !== undefined) {
        return orderA - orderB;
      }
      if (orderA !== undefined) {
        return -1;
      }
      if (orderB !== undefined) {
        return 1;
      }
      return a.index - b.index;
    })
    .map(({ protocol }) => protocol);
}

// A route binding runs exactly one protocol; selecting another replaces the current choice.
export function isSupportedRouteProtocolSelection(protocols: readonly AdminLLMAdapter[]): boolean {
  return new Set(protocols).size <= 1;
}

export function resolveNextRouteProtocolSelection(
  currentProtocols: readonly AdminLLMAdapter[],
  protocol: AdminLLMAdapter,
): AdminLLMAdapter[] {
  const current = sortProtocolsForDisplay(currentProtocols);
  if (current.includes(protocol)) {
    return sortProtocolsForDisplay(current.filter((item) => item !== protocol));
  }
  const candidate = sortProtocolsForDisplay([...current, protocol]);
  if (isSupportedRouteProtocolSelection(candidate)) {
    return candidate;
  }
  return [protocol];
}

export function resolveKindsDisplayForProtocols(
  protocols: readonly AdminLLMAdapter[],
  fallbackDisplay = "chat",
): string {
  const kinds = Array.from(new Set(protocols.flatMap((protocol) => PROTOCOL_KINDS[protocol] ?? [])));
  return kinds.length > 0 ? kinds.join(",") : fallbackDisplay;
}

export function resolveCompatibleLabel(compatible: string): string {
  return COMPATIBLE_OPTIONS.find((item) => item.value === compatible)?.label ?? (compatible || "-");
}
