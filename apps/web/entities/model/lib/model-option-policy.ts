import type { ModelOptionPolicy, ModelOptionPolicyMode } from "@/shared/api/settings-types";
import { isRecord } from "@/shared/lib/type-guards";

// The policy payload is a transport contract owned by shared/api; re-exported so model
// consumers read the whole policy vocabulary from the model entity.
export type { ModelOptionPolicy, ModelOptionPolicyMode };

// Protocol display order is always by vendor: OpenAI → Anthropic → Google → xAI → OpenRouter.
export const MODEL_OPTION_POLICY_PROTOCOLS = [
  "default",
  "openai_chat_completions",
  "openai_responses",
  "anthropic_messages",
  "gemini_generate_content",
  "gemini_interactions",
  "xai_responses",
  "openrouter_chat_completions",
  "openrouter_responses",
] as const;

export type ModelOptionPolicyProtocol = (typeof MODEL_OPTION_POLICY_PROTOCOLS)[number];

export type ModelOptionRuleMap = Partial<Record<ModelOptionPolicyProtocol | string, string[]>>;

export const MODEL_OPTION_POLICY_PROTOCOL_LABELS: Record<ModelOptionPolicyProtocol, string> = {
  default: "Default",
  openai_chat_completions: "OpenAI（Chat Completions）",
  openai_responses: "OpenAI（Responses）",
  anthropic_messages: "Anthropic（Messages）",
  gemini_generate_content: "Google（Generate Content）",
  gemini_interactions: "Google（Interactions）",
  xai_responses: "xAI（Responses）",
  openrouter_chat_completions: "OpenRouter（Chat Completions）",
  openrouter_responses: "OpenRouter（Responses）",
};

export const HARD_DENIED_MODEL_OPTION_PATHS = [
  "model",
  "messages",
  "input",
  "instructions",
  "prompt",
  "system",
  "systemInstruction",
  "headers",
  "api_key",
  "apiKey",
  "base_url",
  "baseURL",
  "stream",
  "previous_response_id",
  "prompt_cache_key",
  "prompt_cache_options",
  "prompt_cache_breakpoint",
  "prompt_cache_retention",
];

export function parseModelOptionRuleMap(raw: string): { value: ModelOptionRuleMap; error: string } {
  try {
    const parsed: unknown = JSON.parse(raw || "{}");
    if (!isRecord(parsed)) {
      return { value: {}, error: "Configuration must be a JSON object" };
    }
    const value: ModelOptionRuleMap = {};
    for (const [key, paths] of Object.entries(parsed)) {
      if (!Array.isArray(paths)) {
        return { value: {}, error: `${key} must be a string array` };
      }
      value[key] = paths.map((path) => (typeof path === "string" ? path.trim() : "")).filter(Boolean);
    }
    return { value, error: "" };
  } catch {
    return { value: {}, error: "Invalid JSON format" };
  }
}

export function uniqueModelOptionPaths(paths: string[]): string[] {
  return [...new Set(paths.map((path) => path.trim()).filter(Boolean))];
}

export function resolveModelOptionPolicyProtocol(protocol: string): ModelOptionPolicyProtocol {
  switch (protocol.trim().toLowerCase()) {
    case "openai":
    case "openai_responses":
      return "openai_responses";
    case "openrouter_chat_completions":
      return "openrouter_chat_completions";
    case "openrouter":
    case "openrouter_responses":
      return "openrouter_responses";
    case "openai_chat_completions":
      return "openai_chat_completions";
    case "anthropic":
    case "claude":
    case "anthropic_messages":
      return "anthropic_messages";
    case "xai":
    case "grok":
    case "xai_responses":
      return "xai_responses";
    case "google":
    case "gemini":
    case "google_generate_content":
    case "gemini_generate_content":
      return "gemini_generate_content";
    case "gemini_interactions":
      return "gemini_interactions";
    default:
      return "openai_responses";
  }
}

export function effectiveModelOptionPaths(rules: ModelOptionRuleMap, protocol: string): string[] {
  if (protocol === "default") {
    return uniqueModelOptionPaths(rules.default ?? []);
  }
  return uniqueModelOptionPaths([...(rules.default ?? []), ...(rules[protocol] ?? [])]);
}

function pathSegments(path: string): string[] {
  return path.split(".").map((item) => item.trim()).filter(Boolean);
}

function ruleAffectsPath(rule: string, path: string): boolean {
  const ruleParts = pathSegments(rule);
  const pathParts = pathSegments(path);
  if (ruleParts.length === 0 || pathParts.length === 0 || ruleParts.length > pathParts.length) {
    return false;
  }
  return ruleParts.every((part, index) => part === pathParts[index]);
}

export function isModelOptionPathFiltered({
  policy,
  protocol,
  path,
}: {
  policy: ModelOptionPolicy;
  protocol: string;
  path: string;
}): boolean {
  const mode = policy.mode?.trim() || "allowlist";
  if (mode === "disabled") {
    return true;
  }

  const policyProtocol = resolveModelOptionPolicyProtocol(protocol);
  const allowed = parseModelOptionRuleMap(policy.allowedPathsJSON).value;
  const denied = parseModelOptionRuleMap(policy.deniedPathsJSON).value;
  const deniedPaths = uniqueModelOptionPaths([
    ...HARD_DENIED_MODEL_OPTION_PATHS,
    ...(mode === "denylist" ? effectiveModelOptionPaths(denied, policyProtocol) : []),
  ]);
  if (deniedPaths.some((rule) => ruleAffectsPath(rule, path))) {
    return true;
  }

  if (mode === "denylist") {
    return false;
  }

  const allowedPaths = effectiveModelOptionPaths(allowed, policyProtocol);
  return !allowedPaths.some((rule) => ruleAffectsPath(rule, path));
}
