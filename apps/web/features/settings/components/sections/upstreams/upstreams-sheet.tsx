"use client";

import { useTranslations } from "next-intl";
import * as React from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { SpinnerLabel } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import { UPSTREAM_COMPATIBLE_OPTIONS } from "@/entities/model";
import type { UserUpstreamDTO } from "@/shared/api/upstreams-types";

export type UpstreamSheetInput = { name: string; baseURL: string; compatible: string; apiKeys: string };

export function UpstreamsSheet({
  open,
  onOpenChange,
  upstream,
  saving,
  onCreate,
  onUpdate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  upstream: UserUpstreamDTO | null;
  saving: boolean;
  onCreate: (input: UpstreamSheetInput) => Promise<boolean>;
  onUpdate: (id: number, input: Partial<UpstreamSheetInput>) => Promise<boolean>;
}) {
  const t = useTranslations("settings.upstreamsPage");
  const id = React.useId();
  const [name, setName] = React.useState("");
  const [baseURL, setBaseURL] = React.useState("");
  const [compatible, setCompatible] = React.useState("openai");
  const [apiKeys, setAPIKeys] = React.useState("");

  React.useEffect(() => {
    if (!open) {
      return;
    }
    setName(upstream?.name ?? "");
    setBaseURL(upstream?.baseURL ?? "");
    setCompatible(upstream?.compatible ?? "openai");
    setAPIKeys("");
  }, [open, upstream]);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const input: Partial<UpstreamSheetInput> = {
      name: name.trim(),
      baseURL: baseURL.trim(),
      compatible,
    };
    const keys = apiKeys.split(/\r?\n/).map((key) => key.trim()).filter(Boolean);
    if (keys.length > 0) {
      input.apiKeys = JSON.stringify({ strategy: "failover", keys: keys.map((key) => ({ key, status: "active" })) });
    }
    const success = upstream
      ? await onUpdate(upstream.id, input)
      : await onCreate({ ...input, name: input.name ?? "", baseURL: input.baseURL ?? "", compatible, apiKeys: input.apiKeys ?? "" });
    if (success) {
      onOpenChange(false);
    }
  }

  return (
    <Sheet open={open} onOpenChange={(nextOpen) => !saving && onOpenChange(nextOpen)}>
      <SheetContent className="flex flex-col gap-0 sm:max-w-[460px]">
        <SheetHeader className="px-4 pb-4">
          <SheetTitle>{upstream ? t("editTitle") : t("createTitle")}</SheetTitle>
          <SheetDescription>{upstream ? t("editDescription") : t("description")}</SheetDescription>
        </SheetHeader>
        <form className="flex min-h-0 flex-1 flex-col" onSubmit={submit}>
          <div className="flex-1 space-y-4 overflow-y-auto px-4 pb-5">
            <div className="space-y-2">
              <Label htmlFor={`${id}-name`}>{t("name")}</Label>
              <Input
                id={`${id}-name`}
                value={name}
                onChange={(event) => setName(event.target.value)}
                disabled={saving}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${id}-url`}>{t("baseURL")}</Label>
              <Input
                id={`${id}-url`}
                type="url"
                value={baseURL}
                onChange={(event) => setBaseURL(event.target.value)}
                className="font-mono text-xs"
                placeholder="https://api.example.com/v1"
                disabled={saving}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${id}-compatible`}>{t("compatible")}</Label>
              <Select value={compatible} onValueChange={setCompatible} disabled={saving}>
                <SelectTrigger id={`${id}-compatible`} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {UPSTREAM_COMPATIBLE_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.value === "custom" ? t("compatibleCustom") : option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${id}-keys`}>{t("apiKey")}</Label>
              <Textarea
                id={`${id}-keys`}
                value={apiKeys}
                onChange={(event) => setAPIKeys(event.target.value)}
                className="h-24 resize-none font-mono text-xs [field-sizing:fixed]"
                placeholder={upstream ? t("apiKeyOptional") : t("apiKeysPlaceholder")}
                disabled={saving}
                required={!upstream}
                spellCheck={false}
                autoComplete="off"
              />
              <p className="text-[11px] text-muted-foreground">{t("apiKeysDescription")}</p>
            </div>
          </div>
          <SheetFooter className="flex flex-row justify-end gap-2 px-4 py-3">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={saving}>
              {t("cancel")}
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? <SpinnerLabel>{upstream ? t("saving") : t("creating")}</SpinnerLabel> : t("save")}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
