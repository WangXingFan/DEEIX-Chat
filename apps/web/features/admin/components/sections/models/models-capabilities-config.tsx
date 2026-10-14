"use client";

import { useState } from "react";
import { CircleHelp, CopyPlus, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogHeightTransition,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";
import type { AdminLLMModelDTO } from "@/features/admin/api/llm-types";
import { ModelCapabilitiesPresetDialog } from "@/features/admin/components/sections/models/models-capabilities-presets";
import { resolveModelOptionPolicyProtocol } from "@/entities/model";
import { isOneOf, isRecord } from "@/shared/lib/type-guards";

export const MODEL_CAPABILITIES_PLACEHOLDER = `{
  "defaultOptions": {},
  "optionControls": [
    {
      "path": "reasoning_effort",
      "label": "Reasoning Effort",
      "description": "Constrains reasoning effort for supported models.",
      "type": "select",
      "options": ["none", "low", "medium", "high"]
    },
    {
      "path": "verbosity",
      "label": "Verbosity",
      "description": "Controls how verbose the response is.",
      "type": "select",
      "options": ["low", "medium", "high"]
    }
  ]
}`;

const CAPABILITY_CONTROL_TYPES = ["text", "select", "number", "boolean"] as const;
type CapabilityControlType = (typeof CAPABILITY_CONTROL_TYPES)[number];
const isCapabilityControlType = isOneOf(CAPABILITY_CONTROL_TYPES);

const PROMPT_CACHE_AVAILABILITIES = ["auto", "enabled", "disabled"] as const;
const isPromptCacheAvailability = isOneOf(PROMPT_CACHE_AVAILABILITIES);
const PROMPT_CACHE_MODES = ["implicit", "explicit"] as const;
const isPromptCacheMode = isOneOf(PROMPT_CACHE_MODES);

type PromptCacheConfig = {
  availability: (typeof PROMPT_CACHE_AVAILABILITIES)[number];
  mode: (typeof PROMPT_CACHE_MODES)[number];
};

type ParameterRow = {
  id: string;
  path: string;
  label: string;
  description: string;
  type: CapabilityControlType;
  options: string;
  defaultValue: string;
  locked: boolean;
};

type CapabilityRowErrors = Record<string, Partial<Record<"path" | "type" | "options" | "defaultValue", string>>>;

const OPENAI_PROMPT_CACHE_PROTOCOLS = new Set(["openai_chat_completions", "openai_responses"]);
const DEFAULT_PROMPT_CACHE_CONFIG: PromptCacheConfig = {
  availability: "auto",
  mode: "implicit",
};

function createCapabilityRowID(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function parseCapabilitiesObject(raw: string): Record<string, unknown> | null {
  const normalized = raw.trim();
  if (!normalized) {
    return {};
  }
  try {
    const parsed: unknown = JSON.parse(normalized);
    return isRecord(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function supportsPromptCacheProtocols(routeProtocols: string[]): boolean {
  return routeProtocols.some((protocol) => OPENAI_PROMPT_CACHE_PROTOCOLS.has(resolveModelOptionPolicyProtocol(protocol)));
}

function parsePromptCacheConfig(value: unknown): PromptCacheConfig {
  if (!isRecord(value)) {
    return DEFAULT_PROMPT_CACHE_CONFIG;
  }
  const availability = value.enabled === true
    ? "enabled"
    : value.enabled === false
      ? "disabled"
      : "auto";
  const mode = value.mode === "explicit" ? "explicit" : "implicit";
  return { availability, mode };
}

function applyPromptCacheConfig(payload: Record<string, unknown>, config: PromptCacheConfig) {
  const promptCache = isRecord(payload.promptCache) ? { ...payload.promptCache } : {};

  if (config.availability === "disabled") {
    promptCache.enabled = false;
    delete promptCache.mode;
    delete promptCache.ttl;
    delete promptCache.retention;
  } else {
    if (config.availability === "enabled") {
      promptCache.enabled = true;
    } else {
      delete promptCache.enabled;
    }

    if (config.mode === "explicit") {
      promptCache.mode = "explicit";
      promptCache.ttl = "30m";
      delete promptCache.retention;
    } else {
      delete promptCache.mode;
      delete promptCache.ttl;
      // Retention is intentionally unsupported. Remove legacy values whenever
      // the visual editor saves a model capability configuration.
      delete promptCache.retention;
    }
  }

  if (Object.keys(promptCache).length > 0) {
    payload.promptCache = promptCache;
  } else {
    delete payload.promptCache;
  }
}

function optionPathSegments(path: string): string[] {
  return path
    .split(".")
    .map((segment) => segment.trim())
    .filter(Boolean);
}

function isValidOptionPathInput(path: string): boolean {
  const normalized = path.trim();
  return Boolean(normalized) && normalized.split(".").every((segment) => segment.trim());
}

function formatDefaultOptionValue(value: unknown): string {
  if (value === undefined) {
    return "";
  }
  return JSON.stringify(value);
}

function flattenDefaultOptions(value: unknown, prefix: string[] = []): { path: string; value: string; rawValue: unknown }[] {
  if (isRecord(value)) {
    return Object.entries(value).flatMap(([key, child]) => flattenDefaultOptions(child, [...prefix, key]));
  }
  if (prefix.length === 0) {
    return [];
  }
  return [{
    path: prefix.join("."),
    rawValue: value,
    value: formatDefaultOptionValue(value),
  }];
}

function parseDefaultOptionValue(value: string): unknown {
  const normalized = value.trim();
  if (!normalized) {
    return null;
  }
  try {
    return JSON.parse(normalized);
  } catch {
    return normalized;
  }
}

function setNestedOptionValue(target: Record<string, unknown>, path: string[], value: unknown) {
  if (path.length === 0) {
    return;
  }
  let current = target;
  path.slice(0, -1).forEach((segment) => {
    const existing = current[segment];
    const nextValue: Record<string, unknown> = isRecord(existing) ? existing : {};
    if (!isRecord(existing)) {
      current[segment] = nextValue;
    }
    current = nextValue;
  });
  current[path[path.length - 1]] = value;
}

function buildDefaultOptions(rows: ParameterRow[]): Record<string, unknown> {
  const options: Record<string, unknown> = {};
  rows.forEach((row) => {
    if (!row.defaultValue.trim()) {
      return;
    }
    const path = optionPathSegments(row.path);
    if (path.length === 0) {
      return;
    }
    setNestedOptionValue(options, path, parseDefaultOptionValue(row.defaultValue));
  });
  return options;
}

function parseLockedOptionPaths(value: unknown): Set<string> {
  if (!Array.isArray(value)) {
    return new Set();
  }
  return new Set(
    value
      .map((item) => (typeof item === "string" ? optionPathSegments(item).join(".") : ""))
      .filter(Boolean),
  );
}

function normalizeControlType(value: unknown): CapabilityControlType {
  return isCapabilityControlType(value) ? value : "text";
}

function inferControlType(value: unknown): CapabilityControlType {
  if (typeof value === "boolean") {
    return "boolean";
  }
  if (typeof value === "number") {
    return "number";
  }
  return "text";
}

function parseOptionControls(value: unknown): ParameterRow[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.flatMap((item): ParameterRow[] => {
    if (!isRecord(item) || typeof item.path !== "string") {
      return [];
    }
    const path = optionPathSegments(item.path);
    if (path.length === 0) {
      return [];
    }
    const options = Array.isArray(item.options)
      ? item.options.map((option) => (typeof option === "string" ? option.trim() : "")).filter(Boolean).join(", ")
      : "";
    return [{
      id: createCapabilityRowID(),
      path: path.join("."),
      label: typeof item.label === "string" ? item.label : "",
      description: typeof item.description === "string" ? item.description : "",
      type: normalizeControlType(item.type),
      options,
      defaultValue: "",
      locked: false,
    }];
  });
}

function parseParameterRows(defaultOptions: unknown, optionControls: unknown, lockedOptionPaths: unknown): ParameterRow[] {
  const lockedPaths = parseLockedOptionPaths(lockedOptionPaths);
  const defaultsByPath = new Map<string, { value: string; rawValue: unknown }>();
  flattenDefaultOptions(defaultOptions).forEach((item) => {
    defaultsByPath.set(item.path, { value: item.value, rawValue: item.rawValue });
  });
  const rows = parseOptionControls(optionControls).map((row) => {
    const defaultItem = defaultsByPath.get(row.path);
    defaultsByPath.delete(row.path);
    return {
      ...row,
      defaultValue: defaultItem?.value ?? "",
      locked: lockedPaths.has(row.path),
    };
  });
  defaultsByPath.forEach((item, path) => {
    rows.push({
      id: createCapabilityRowID(),
      path,
      label: "",
      description: "",
      type: inferControlType(item.rawValue),
      options: "",
      defaultValue: item.value,
      locked: lockedPaths.has(path),
    });
  });
  return rows;
}

function parseControlOptions(value: string): string[] {
  const normalized = value.trim();
  if (!normalized) {
    return [];
  }
  return Array.from(
    new Set(
      normalized
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean),
    ),
  );
}

function buildOptionControls(rows: ParameterRow[]): Record<string, unknown>[] {
  return rows.flatMap((row): Record<string, unknown>[] => {
    const path = optionPathSegments(row.path);
    if (path.length === 0) {
      return [];
    }
    const control: Record<string, unknown> = {
      path: path.join("."),
      type: row.type,
    };
    const label = row.label.trim();
    const description = row.description.trim();
    const options = parseControlOptions(row.options);
    if (label) {
      control.label = label;
    }
    if (description) {
      control.description = description;
    }
    if (row.type === "select" && options.length > 0) {
      control.options = options;
    }
    return [control];
  });
}

function buildLockedOptionPaths(rows: ParameterRow[]): string[] {
  return Array.from(new Set(rows.flatMap((row) => {
    if (!row.locked || !row.defaultValue.trim()) {
      return [];
    }
    const path = optionPathSegments(row.path);
    return path.length > 0 ? [path.join(".")] : [];
  })));
}

function markDuplicatePathErrors<T extends { id: string; path: string }>(
  rows: T[],
  errors: CapabilityRowErrors,
  message: string,
) {
  const rowsByPath = new Map<string, T[]>();
  rows.forEach((row) => {
    if (!isValidOptionPathInput(row.path)) {
      return;
    }
    const normalizedPath = optionPathSegments(row.path).join(".");
    rowsByPath.set(normalizedPath, [...(rowsByPath.get(normalizedPath) ?? []), row]);
  });
  rowsByPath.forEach((items) => {
    if (items.length < 2) {
      return;
    }
    items.forEach((item) => {
      errors[item.id] = { ...(errors[item.id] ?? {}), path: message };
    });
  });
}

function validateParameterRows(rows: ParameterRow[], t: (key: string) => string): CapabilityRowErrors {
  const errors: CapabilityRowErrors = {};
  rows.forEach((row) => {
    if (!isValidOptionPathInput(row.path)) {
      errors[row.id] = { ...(errors[row.id] ?? {}), path: t("sheet.capabilitiesQuick.pathRequired") };
    }
    if (!CAPABILITY_CONTROL_TYPES.includes(row.type)) {
      errors[row.id] = { ...(errors[row.id] ?? {}), type: t("sheet.capabilitiesQuick.typeRequired") };
    }
    if (row.type === "select" && parseControlOptions(row.options).length === 0) {
      errors[row.id] = { ...(errors[row.id] ?? {}), options: t("sheet.capabilitiesQuick.selectOptionsRequired") };
    }
    if (row.locked && !row.defaultValue.trim()) {
      errors[row.id] = { ...(errors[row.id] ?? {}), defaultValue: t("sheet.capabilitiesQuick.lockedDefaultRequired") };
    }
  });
  markDuplicatePathErrors(rows, errors, t("sheet.capabilitiesQuick.duplicatePath"));
  return errors;
}

function hasCapabilityErrors(errors: CapabilityRowErrors): boolean {
  return Object.values(errors).some((rowErrors) => Object.keys(rowErrors).length > 0);
}

export function normalizeModelCapabilitiesJSON(value: string | null | undefined): string {
  const trimmed = value?.trim() ?? "";
  if (!trimmed || trimmed === "{}") {
    return "";
  }
  const payload = parseCapabilitiesObject(trimmed);
  if (!payload) {
    return trimmed;
  }
  delete payload.nativeTools;
  delete payload.nativeToolKeys;
  return Object.keys(payload).length > 0 ? JSON.stringify(payload, null, 2) : "";
}

function buildCapabilitiesJSON(
  currentJSON: string,
  parameterRows: ParameterRow[],
  promptCacheConfig: PromptCacheConfig,
  promptCacheSupported: boolean,
): string | null {
  const payload = parseCapabilitiesObject(currentJSON);
  if (!payload) {
    return null;
  }
  const defaultOptions = buildDefaultOptions(parameterRows);
  const optionControls = buildOptionControls(parameterRows);
  const lockedOptionPaths = buildLockedOptionPaths(parameterRows);
  if (Object.keys(defaultOptions).length > 0) {
    payload.defaultOptions = defaultOptions;
  } else {
    delete payload.defaultOptions;
  }
  if (optionControls.length > 0) {
    payload.optionControls = optionControls;
  } else {
    delete payload.optionControls;
  }
  if (lockedOptionPaths.length > 0) {
    payload.lockedOptionPaths = lockedOptionPaths;
  } else {
    delete payload.lockedOptionPaths;
  }
  if (promptCacheSupported) {
    applyPromptCacheConfig(payload, promptCacheConfig);
  }
  delete payload.nativeTools;
  delete payload.nativeToolKeys;
  return Object.keys(payload).length > 0 ? JSON.stringify(payload, null, 2) : "";
}

export function ModelCapabilitiesGuideButton({ t }: { t: (key: string) => string }) {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button type="button" variant="link" size="sm" className="h-auto gap-1 px-0 py-0 text-[11px] font-normal text-muted-foreground">
          <CircleHelp className="size-3.5" />
          {t("sheet.capabilitiesGuide.button")}
        </Button>
      </DialogTrigger>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-[760px]">
        <DialogHeightTransition contentClassName="max-h-[min(86vh,760px)]">
          <DialogHeader className="shrink-0 px-4 py-4">
            <DialogTitle>{t("sheet.capabilitiesGuide.title")}</DialogTitle>
            <DialogDescription>{t("sheet.capabilitiesGuide.description")}</DialogDescription>
          </DialogHeader>

          <Tabs defaultValue="defaults" className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden px-4 py-2">
            <TabsList className="shrink-0">
              <TabsTrigger value="defaults">{t("sheet.capabilitiesGuide.defaultsTab")}</TabsTrigger>
              <TabsTrigger value="controls">{t("sheet.capabilitiesGuide.controlsTab")}</TabsTrigger>
              <TabsTrigger value="policy">{t("sheet.capabilitiesGuide.policyTab")}</TabsTrigger>
            </TabsList>

            <TabsContent value="defaults" className="min-h-0 flex-1 space-y-3 overflow-y-auto text-sm text-muted-foreground">
              <p className="text-xs">{t("sheet.capabilitiesGuide.defaultsDescription")}</p>
              <pre className="max-h-72 overflow-auto rounded-md bg-muted/50 p-3 text-xs text-foreground">
  {`{
    "defaultOptions": {
      "store": false,
      "reasoning": {
        "effort": "medium"
      },
      "text": {
        "verbosity": "medium"
      }
    }
  }`}
              </pre>
            </TabsContent>

            <TabsContent value="controls" className="min-h-0 flex-1 space-y-3 overflow-y-auto text-sm text-muted-foreground">
              <p className="text-xs">{t("sheet.capabilitiesGuide.controlsDescription")}</p>
              <pre className="max-h-72 overflow-auto rounded-md bg-muted/50 p-3 text-xs text-foreground">
  {`{
    "defaultOptions": {},
    "optionControls": [
      {
        "path": "reasoning_effort",
        "label": "Reasoning Effort",
        "description": "Constrains reasoning effort for supported models.",
        "type": "select",
        "options": ["none", "low", "medium", "high"]
      },
      {
        "path": "temperature",
        "label": "Temperature",
        "type": "number",
        "placeholder": "1"
      }
    ]
  }`}
              </pre>
              <p className="text-xs">{t("sheet.capabilitiesGuide.controlTypes")}</p>
            </TabsContent>

            <TabsContent value="policy" className="min-h-0 flex-1 space-y-3 overflow-y-auto text-sm text-muted-foreground">
              <p className="text-xs">{t("sheet.capabilitiesGuide.policyDescription")}</p>
              <pre className="max-h-72 overflow-auto rounded-md bg-muted/50 p-3 text-xs text-foreground">
  {`{
    "openai_chat_completions": [
      "reasoning_effort",
      "verbosity"
    ],
    "xai_responses": [
      "reasoning.effort",
      "top_k"
    ]
  }`}
              </pre>
            </TabsContent>
          </Tabs>
        </DialogHeightTransition>
      </DialogContent>
    </Dialog>
  );
}

export function ModelCapabilitiesQuickConfig({
  value,
  disabled,
  presetModels = [],
  currentModelID,
  routeProtocols,
  t,
  commonT,
  triggerVariant = "ghost",
  triggerClassName,
  triggerLabel,
  onApply,
}: {
  value: string;
  disabled: boolean;
  presetModels?: AdminLLMModelDTO[];
  currentModelID?: number | null;
  routeProtocols: string[];
  t: (key: string, values?: Record<string, string | number>) => string;
  commonT: (key: string) => string;
  triggerVariant?: "default" | "secondary" | "ghost" | "outline" | "link";
  triggerClassName?: string;
  triggerLabel?: string;
  onApply: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [presetOpen, setPresetOpen] = useState(false);
  const [draftBaseJSON, setDraftBaseJSON] = useState("");
  const [parameterRows, setParameterRows] = useState<ParameterRow[]>([]);
  const [promptCacheConfig, setPromptCacheConfig] = useState<PromptCacheConfig>(DEFAULT_PROMPT_CACHE_CONFIG);
  const [parameterErrors, setParameterErrors] = useState<CapabilityRowErrors>({});
  const promptCacheSupported = supportsPromptCacheProtocols(routeProtocols);

  function loadDraft() {
    const payload = parseCapabilitiesObject(value);
    if (!payload) {
      toast.error(t("sheet.capabilitiesQuick.invalidJSON"));
      return false;
    }
    setParameterRows(parseParameterRows(payload.defaultOptions, payload.optionControls, payload.lockedOptionPaths));
    setPromptCacheConfig(parsePromptCacheConfig(payload.promptCache));
    setParameterErrors({});
    setDraftBaseJSON(value);
    return true;
  }

  function openDialog() {
    if (!loadDraft()) {
      return;
    }
    setOpen(true);
  }

  function updateParameterRow(id: string, patch: Partial<ParameterRow>) {
    setParameterErrors((prev) => {
      const { [id]: _rowErrors, ...rest } = prev;
      return rest;
    });
    setParameterRows((prev) => prev.map((row) => (row.id === id ? { ...row, ...patch } : row)));
  }

  function updateParameterType(id: string, type: CapabilityControlType) {
    setParameterErrors((prev) => {
      const { [id]: _rowErrors, ...rest } = prev;
      return rest;
    });
    setParameterRows((prev) => prev.map((row) => (
      row.id === id ? { ...row, type, options: type === "select" ? row.options : "" } : row
    )));
  }

  function addParameterRow() {
    setParameterRows((prev) => [{
      id: createCapabilityRowID(),
      path: "",
      label: "",
      description: "",
      type: "text",
      options: "",
      defaultValue: "",
      locked: false,
    }, ...prev]);
  }

  function applyDraft() {
    const nextParameterErrors = validateParameterRows(parameterRows, t);
    setParameterErrors(nextParameterErrors);
    if (hasCapabilityErrors(nextParameterErrors)) {
      toast.error(t("sheet.capabilitiesQuick.validationFailed"));
      return;
    }
    const nextValue = buildCapabilitiesJSON(
      draftBaseJSON,
      parameterRows,
      promptCacheConfig,
      promptCacheSupported,
    );
    if (nextValue === null) {
      toast.error(t("sheet.capabilitiesQuick.invalidJSON"));
      return;
    }
    onApply(nextValue);
    setOpen(false);
  }

  function applyPresetValue(nextValue: string) {
    const payload = parseCapabilitiesObject(nextValue);
    if (!payload) {
      toast.error(t("sheet.capabilitiesQuick.invalidJSON"));
      return;
    }
    // Automatic context windows belong to the source model identity. A preset
    // may be applied to a different model, so let the destination resolve its
    // own catalog value instead of copying a cached inference.
    if (payload._deeixContextWindowMode === "auto") {
      delete payload.contextWindow;
      delete payload._deeixContextWindowMode;
    }
    const sanitizedValue = Object.keys(payload).length > 0 ? JSON.stringify(payload, null, 2) : "";
    setParameterRows(parseParameterRows(payload.defaultOptions, payload.optionControls, payload.lockedOptionPaths));
    setPromptCacheConfig(parsePromptCacheConfig(payload.promptCache));
    setParameterErrors({});
    setDraftBaseJSON(sanitizedValue);
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button
        type="button"
        variant={triggerVariant}
        size="sm"
        className={cn("h-6 px-2 text-[11px]", triggerClassName)}
        disabled={disabled}
        onClick={openDialog}
      >
        {triggerLabel ?? t("sheet.capabilitiesQuick.button")}
      </Button>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-[760px]">
        <DialogHeightTransition contentClassName="h-[min(86vh,760px)] max-h-[min(86vh,760px)]">
          <DialogHeader className="shrink-0 px-4 py-4">
            <div className="flex min-w-0 items-start justify-between gap-3">
              <div className="min-w-0 space-y-1.5">
                <DialogTitle>{t("sheet.capabilitiesQuick.title")}</DialogTitle>
                <DialogDescription>{t("sheet.capabilitiesQuick.description")}</DialogDescription>
              </div>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="h-7 shrink-0 gap-1 px-2 text-xs font-normal shadow-none"
                onClick={() => setPresetOpen(true)}
              >
                <CopyPlus className="size-3.5" />
                {t("sheet.capabilitiesPreset.button")}
              </Button>
            </div>
          </DialogHeader>

          <ModelCapabilitiesPresetDialog
            open={presetOpen}
            onOpenChange={setPresetOpen}
            models={presetModels}
            currentModelID={currentModelID}
            routeProtocols={routeProtocols}
            t={t}
            commonT={commonT}
            onApply={applyPresetValue}
          />

          <div className="min-h-0 min-w-0 flex flex-1 flex-col overflow-hidden px-4 py-2">
            <div className="flex min-h-0 min-w-0 flex-1 flex-col gap-3 overflow-hidden">
              <div className="min-h-0 flex flex-1 flex-col gap-3 overflow-hidden pr-1">
                <div className="flex min-w-0 shrink-0 items-start justify-between gap-3">
                  <p className="min-w-0 flex-1 text-xs leading-5 text-muted-foreground">
                    {t("sheet.capabilitiesQuick.parametersIntro")}
                  </p>
                  <Button
                    type="button"
                    variant="default"
                    size="sm"
                    className="h-7 shrink-0 whitespace-nowrap px-2 text-xs"
                    onClick={addParameterRow}
                  >
                    <Plus className="size-3.5" />
                    {t("sheet.capabilitiesQuick.addParameter")}
                  </Button>
                </div>
                {promptCacheSupported ? (
                  <div className="shrink-0 space-y-3 rounded-md border bg-muted/20 px-3 py-3">
                    <div className="min-w-0 space-y-0.5">
                      <p className="text-xs font-medium text-foreground/85">
                        {t("sheet.capabilitiesQuick.promptCacheTitle")}
                      </p>
                      <p className="text-[11px] leading-4 text-muted-foreground">
                        {t("sheet.capabilitiesQuick.promptCacheDescription")}
                      </p>
                    </div>
                    <div className={cn(
                      "grid min-w-0 grid-cols-1 gap-2",
                      promptCacheConfig.mode === "explicit" ? "sm:grid-cols-3" : "sm:grid-cols-2",
                    )}>
                      <label className="min-w-0 space-y-1">
                        <span className="block truncate px-1 text-[11px] text-muted-foreground">
                          {t("sheet.capabilitiesQuick.promptCacheAvailability")}
                        </span>
                        <Select
                          value={promptCacheConfig.availability}
                          onValueChange={(availability) => {
                            if (!isPromptCacheAvailability(availability)) return;
                            setPromptCacheConfig((current) => ({ ...current, availability }));
                          }}
                        >
                          <SelectTrigger className="h-8 w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="auto">{t("sheet.capabilitiesQuick.promptCacheAuto")}</SelectItem>
                            <SelectItem value="enabled">{t("sheet.capabilitiesQuick.promptCacheEnabled")}</SelectItem>
                            <SelectItem value="disabled">{t("sheet.capabilitiesQuick.promptCacheDisabled")}</SelectItem>
                          </SelectContent>
                        </Select>
                      </label>
                      <label className="min-w-0 space-y-1">
                        <span className="block truncate px-1 text-[11px] text-muted-foreground">
                          {t("sheet.capabilitiesQuick.promptCacheMode")}
                        </span>
                        <Select
                          value={promptCacheConfig.mode}
                          disabled={promptCacheConfig.availability === "disabled"}
                          onValueChange={(mode) => {
                            if (!isPromptCacheMode(mode)) return;
                            setPromptCacheConfig((current) => ({ ...current, mode }));
                          }}
                        >
                          <SelectTrigger className="h-8 w-full">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="implicit">{t("sheet.capabilitiesQuick.promptCacheImplicit")}</SelectItem>
                            <SelectItem value="explicit">{t("sheet.capabilitiesQuick.promptCacheExplicit")}</SelectItem>
                          </SelectContent>
                        </Select>
                      </label>
                      {promptCacheConfig.mode === "explicit" ? (
                        <label className="min-w-0 space-y-1">
                          <span className="block truncate px-1 text-[11px] text-muted-foreground">
                            {t("sheet.capabilitiesQuick.promptCacheTTL")}
                          </span>
                          <Select value="30m" disabled>
                            <SelectTrigger className="h-8 w-full">
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value="30m">30m</SelectItem>
                            </SelectContent>
                          </Select>
                        </label>
                      ) : null}
                    </div>
                  </div>
                ) : null}
                {parameterRows.length === 0 ? (
                  <div className="flex min-h-0 flex-1 flex-col items-center justify-center rounded-md border border-dashed px-3 py-8 text-center">
                    <p className="text-xs text-muted-foreground">{t("sheet.capabilitiesQuick.emptyParameters")}</p>
                  </div>
                ) : (
                  <div className="min-h-0 flex-1 overflow-y-auto rounded-md border border-dashed p-3">
                    <div className="min-w-0 space-y-2">
                      {parameterRows.map((row) => {
                        const rowErrors = parameterErrors[row.id] ?? {};
                        return (
                          <div key={row.id} className="grid min-w-0 grid-cols-[minmax(0,1fr)_32px] items-start gap-2 rounded-md bg-muted/40 px-2 py-2">
                            <div className="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-3">
                              <label className="min-w-0 space-y-1">
                                <span className="block truncate px-1 text-[11px] text-muted-foreground">
                                  {t("sheet.capabilitiesQuick.pathColumn")} *
                                </span>
                                <Input
                                  aria-invalid={Boolean(rowErrors.path)}
                                  className={cn("h-8", rowErrors.path && "border-destructive focus-visible:ring-destructive/30")}
                                  value={row.path}
                                  placeholder="path.to.option"
                                  onChange={(event) => updateParameterRow(row.id, { path: event.target.value })}
                                />
                                {rowErrors.path ? <p className="truncate px-1 text-[10px] text-destructive">{rowErrors.path}</p> : null}
                              </label>
                              <label className="min-w-0 space-y-1">
                                <span className="block truncate px-1 text-[11px] text-muted-foreground">
                                  {t("sheet.capabilitiesQuick.labelColumn")}
                                </span>
                                <Input
                                  className="h-8"
                                  value={row.label}
                                  placeholder={t("sheet.capabilitiesQuick.labelPlaceholder")}
                                  onChange={(event) => updateParameterRow(row.id, { label: event.target.value })}
                                />
                              </label>
                              <label className="min-w-0 space-y-1">
                                <span className="block truncate px-1 text-[11px] text-muted-foreground">
                                  {t("sheet.capabilitiesQuick.descriptionColumn")}
                                </span>
                                <Input
                                  className="h-8"
                                  value={row.description}
                                  placeholder={t("sheet.capabilitiesQuick.descriptionPlaceholder")}
                                  onChange={(event) => updateParameterRow(row.id, { description: event.target.value })}
                                />
                              </label>
                              <label className="min-w-0 space-y-1">
                                <span className="block truncate px-1 text-[11px] text-muted-foreground">
                                  {t("sheet.capabilitiesQuick.typeColumn")} *
                                </span>
                                <Select
                                  value={row.type}
                                  onValueChange={(type) => {
                                    if (isCapabilityControlType(type)) updateParameterType(row.id, type);
                                  }}
                                >
                                  <SelectTrigger
                                    aria-invalid={Boolean(rowErrors.type)}
                                    className={cn("h-8 w-full", rowErrors.type && "border-destructive focus-visible:ring-destructive/30")}
                                  >
                                    <SelectValue />
                                  </SelectTrigger>
                                  <SelectContent>
                                    {CAPABILITY_CONTROL_TYPES.map((type) => (
                                      <SelectItem key={type} value={type}>
                                        {type}
                                      </SelectItem>
                                    ))}
                                  </SelectContent>
                                </Select>
                                {rowErrors.type ? <p className="truncate px-1 text-[10px] text-destructive">{rowErrors.type}</p> : null}
                              </label>
                              <label className="min-w-0 space-y-1">
                                <span className="block truncate px-1 text-[11px] text-muted-foreground">
                                  {t("sheet.capabilitiesQuick.optionsColumn")}{row.type === "select" ? " *" : ""}
                                </span>
                                <Input
                                  aria-invalid={Boolean(rowErrors.options)}
                                  className={cn("h-8", rowErrors.options && "border-destructive focus-visible:ring-destructive/30")}
                                  value={row.options}
                                  disabled={row.type !== "select"}
                                  placeholder={t("sheet.capabilitiesQuick.optionsPlaceholder")}
                                  onChange={(event) => updateParameterRow(row.id, { options: event.target.value })}
                                />
                                {rowErrors.options ? <p className="truncate px-1 text-[10px] text-destructive">{rowErrors.options}</p> : null}
                              </label>
                              <div className="min-w-0 space-y-1">
                                <div className="flex min-w-0 items-center justify-between gap-2 px-1">
                                  <span className="min-w-0 truncate text-[11px] text-muted-foreground">
                                    {t("sheet.capabilitiesQuick.defaultValueColumn")}
                                  </span>
                                  <label className="flex shrink-0 items-center gap-1.5 text-[11px] text-muted-foreground">
                                    <Checkbox
                                      className="size-3.5"
                                      checked={row.locked}
                                      onCheckedChange={(checked) => updateParameterRow(row.id, { locked: checked === true })}
                                    />
                                    <span>{t("sheet.capabilitiesQuick.lockedColumn")}</span>
                                  </label>
                                </div>
                                <Input
                                  aria-invalid={Boolean(rowErrors.defaultValue)}
                                  className={cn("h-8", rowErrors.defaultValue && "border-destructive focus-visible:ring-destructive/30")}
                                  value={row.defaultValue}
                                  placeholder={'"high", 0.7, true, null, {"key":"value"}'}
                                  onChange={(event) => updateParameterRow(row.id, { defaultValue: event.target.value })}
                                />
                                {rowErrors.defaultValue ? <p className="truncate px-1 text-[10px] text-destructive">{rowErrors.defaultValue}</p> : null}
                              </div>
                            </div>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="mt-5 size-8 justify-self-end text-muted-foreground hover:text-destructive"
                              onClick={() => {
                                setParameterRows((prev) => prev.filter((item) => item.id !== row.id));
                                setParameterErrors((prev) => {
                                  const { [row.id]: _rowErrors, ...rest } = prev;
                                  return rest;
                                });
                              }}
                              aria-label={commonT("actions.delete")}
                            >
                              <Trash2 className="size-3.5" />
                            </Button>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>

          <DialogFooter className="shrink-0 px-4 py-3">
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
              {commonT("actions.cancel")}
            </Button>
            <Button type="button" onClick={applyDraft}>
              {commonT("actions.confirm")}
            </Button>
          </DialogFooter>
        </DialogHeightTransition>
      </DialogContent>
    </Dialog>
  );
}
