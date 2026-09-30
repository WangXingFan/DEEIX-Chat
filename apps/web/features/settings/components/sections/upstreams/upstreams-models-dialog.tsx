"use client";

import { Plus, RefreshCw, Search, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SpinnerLabel } from "@/components/ui/spinner";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import type { UserRemoteModelDTO, UserUpstreamDTO, UserUpstreamModelDTO } from "@/shared/api/upstreams-types";
import { UpstreamModelsDialogContent } from "@/shared/components/upstream-models-dialog-content";

function modelAvailable(model: UserUpstreamModelDTO): boolean {
  return model.routeStatus === "active" && model.upstreamModelStatus === "active" && !model.circuitOpen;
}

export function UpstreamsModelsDialog({
  open,
  onOpenChange,
  upstream,
  addedModels,
  saving,
  onDiscover,
  onAddModels,
  onRemoveModel,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  upstream: UserUpstreamDTO | null;
  addedModels: UserUpstreamModelDTO[];
  saving: boolean;
  onDiscover: (upstreamID: number) => Promise<UserRemoteModelDTO[]>;
  onAddModels: (upstreamID: number, modelNames: string[]) => Promise<boolean>;
  onRemoveModel: (upstreamID: number, routeID: number) => Promise<boolean>;
}) {
  const t = useTranslations("settings.upstreamsPage");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const [remoteModels, setRemoteModels] = React.useState<UserRemoteModelDTO[] | null>(null);
  const [remoteLoading, setRemoteLoading] = React.useState(false);
  const [remoteError, setRemoteError] = React.useState("");
  const [remoteQuery, setRemoteQuery] = React.useState("");
  const [addedQuery, setAddedQuery] = React.useState("");
  const [selected, setSelected] = React.useState<Set<string>>(new Set());
  const [manualInput, setManualInput] = React.useState("");
  const upstreamID = upstream?.id ?? 0;

  const loadRemoteModels = React.useCallback(async (id: number) => {
    setRemoteLoading(true);
    setRemoteError("");
    try {
      const items = await onDiscover(id);
      setRemoteModels(items);
      setSelected(new Set());
    } catch (error) {
      setRemoteModels(null);
      setRemoteError(resolveErrorMessage(error));
    } finally {
      setRemoteLoading(false);
    }
  }, [onDiscover, resolveErrorMessage]);

  React.useEffect(() => {
    if (!open) {
      return;
    }
    setRemoteQuery("");
    setAddedQuery("");
    setManualInput("");
    setSelected(new Set());
    if (upstreamID > 0) {
      void loadRemoteModels(upstreamID);
    }
  }, [loadRemoteModels, open, upstreamID]);

  const filteredAdded = React.useMemo(() => {
    const keyword = addedQuery.trim().toLowerCase();
    if (!keyword) {
      return addedModels;
    }
    return addedModels.filter((model) => model.upstreamModelName.toLowerCase().includes(keyword));
  }, [addedModels, addedQuery]);

  const filteredRemote = React.useMemo(() => {
    const keyword = remoteQuery.trim().toLowerCase();
    const items = remoteModels ?? [];
    if (!keyword) {
      return items;
    }
    return items.filter((model) => model.upstreamModelName.toLowerCase().includes(keyword));
  }, [remoteModels, remoteQuery]);

  const selectedNames = React.useMemo(() => Array.from(selected), [selected]);

  function toggleRemote(model: UserRemoteModelDTO, checked: boolean) {
    setSelected((current) => {
      const next = new Set(current);
      if (checked) {
        next.add(model.upstreamModelName);
      } else {
        next.delete(model.upstreamModelName);
      }
      return next;
    });
  }

  async function submitSelection() {
    if (upstreamID === 0 || selectedNames.length === 0) {
      return;
    }
    const success = await onAddModels(upstreamID, selectedNames);
    if (success) {
      setSelected(new Set());
      await loadRemoteModels(upstreamID);
    }
  }

  async function submitManual() {
    const names = manualInput.split(",").map((name) => name.trim()).filter(Boolean);
    if (upstreamID === 0 || names.length === 0) {
      return;
    }
    const success = await onAddModels(upstreamID, names);
    if (success) {
      setManualInput("");
      await loadRemoteModels(upstreamID);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => !saving && onOpenChange(nextOpen)}>
      <UpstreamModelsDialogContent>
        <DialogHeader className="shrink-0 px-4 py-4">
          <DialogTitle>{t("manageModelsTitle", { name: upstream?.name ?? "" })}</DialogTitle>
          <DialogDescription>{t("discoverDescription")}</DialogDescription>
        </DialogHeader>
        <div className="grid min-h-0 flex-1 gap-4 overflow-y-auto px-4 pb-4 md:grid-cols-2">
          <section className="flex min-h-0 flex-col gap-2 rounded-lg border border-border/60 p-3">
            <div className="flex items-center justify-between gap-2">
              <h4 className="text-xs font-medium">{t("addedModels", { count: addedModels.length })}</h4>
            </div>
            <div className="relative">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 stroke-1 text-muted-foreground" />
              <Input
                value={addedQuery}
                onChange={(event) => setAddedQuery(event.target.value)}
                placeholder={t("searchModels")}
                className="h-8 pl-8 text-xs"
              />
            </div>
            <div className="min-h-0 flex-1 space-y-1 overflow-y-auto md:max-h-[320px]">
              {addedModels.length === 0 ? (
                <p className="px-1 py-3 text-[11px] leading-4 text-muted-foreground">{t("noAddedModels")}</p>
              ) : filteredAdded.length === 0 ? (
                <p className="px-1 py-3 text-[11px] leading-4 text-muted-foreground">{t("noMatchingModels")}</p>
              ) : (
                filteredAdded.map((model) => (
                  <div
                    key={model.routeID}
                    className="flex items-center gap-2 rounded-md px-1 py-1.5 hover:bg-muted/60"
                  >
                    <span className="min-w-0 flex-1 truncate text-xs" title={model.upstreamModelName}>
                      {model.upstreamModelName}
                    </span>
                    <Badge variant={modelAvailable(model) ? "secondary" : "outline"} className="shrink-0 text-[10px]">
                      {modelAvailable(model) ? t("available") : t("unavailable")}
                    </Badge>
                    <Button
                      type="button"
                      size="icon-xs"
                      variant="ghost"
                      className="shrink-0 text-muted-foreground"
                      disabled={saving}
                      aria-label={t("removeSelection", { model: model.upstreamModelName })}
                      onClick={() => void onRemoveModel(upstreamID, model.routeID)}
                    >
                      <Trash2 className="size-3.5 stroke-1" />
                    </Button>
                  </div>
                ))
              )}
            </div>
          </section>

          <section className="flex min-h-0 flex-col gap-2 rounded-lg border border-border/60 p-3">
            <div className="flex items-center justify-between gap-2">
              <h4 className="text-xs font-medium">{t("discover")}</h4>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="h-7 gap-1 text-xs"
                disabled={saving || remoteLoading || upstreamID === 0}
                onClick={() => void loadRemoteModels(upstreamID)}
              >
                <RefreshCw className={remoteLoading ? "size-3.5 stroke-1 animate-spin" : "size-3.5 stroke-1"} />
                {t("discover")}
              </Button>
            </div>
            <div className="relative">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 stroke-1 text-muted-foreground" />
              <Input
                value={remoteQuery}
                onChange={(event) => setRemoteQuery(event.target.value)}
                placeholder={t("searchModels")}
                className="h-8 pl-8 text-xs"
                disabled={remoteLoading}
              />
            </div>
            <div className="min-h-0 flex-1 space-y-1 overflow-y-auto md:max-h-[280px]">
              {remoteLoading ? (
                <div className="px-1 py-3 text-[11px] text-muted-foreground">{t("loading")}</div>
              ) : remoteError ? (
                <div className="space-y-2 px-1 py-3">
                  <p className="text-[11px] leading-4 text-muted-foreground">{remoteError}</p>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="h-7 text-xs"
                    onClick={() => void loadRemoteModels(upstreamID)}
                  >
                    {t("retry")}
                  </Button>
                </div>
              ) : remoteModels === null ? (
                <p className="px-1 py-3 text-[11px] leading-4 text-muted-foreground">{t("noRemoteModels")}</p>
              ) : filteredRemote.length === 0 ? (
                <p className="px-1 py-3 text-[11px] leading-4 text-muted-foreground">{t("noMatchingModels")}</p>
              ) : (
                filteredRemote.map((model) => {
                  const checked = model.alreadyBound || selected.has(model.upstreamModelName);
                  return (
                    <label
                      key={model.upstreamModelName}
                      className="flex cursor-pointer items-center gap-2 rounded-md px-1 py-1.5 text-xs hover:bg-muted/60"
                    >
                      <Checkbox
                        checked={checked}
                        disabled={model.alreadyBound || saving}
                        onCheckedChange={(value) => toggleRemote(model, value === true)}
                      />
                      <span className="min-w-0 flex-1 truncate" title={model.upstreamModelName}>
                        {model.upstreamModelName}
                      </span>
                      {model.alreadyBound ? (
                        <span className="shrink-0 text-[10px] text-muted-foreground">{t("added")}</span>
                      ) : null}
                    </label>
                  );
                })
              )}
            </div>
            <div className="flex items-center justify-between gap-2">
              <span className="text-[11px] text-muted-foreground">
                {selectedNames.length > 0 ? t("selectedModels", { count: selectedNames.length }) : t("noSelectedModels")}
              </span>
              <Button
                type="button"
                size="sm"
                className="h-7 gap-1 text-xs"
                disabled={saving || selectedNames.length === 0}
                onClick={() => void submitSelection()}
              >
                <Plus className="size-3.5 stroke-1" />
                {t("addSelected")}
              </Button>
            </div>
          </section>

          <div className="flex flex-col gap-2 rounded-lg border border-border/60 p-3 md:col-span-2">
            <h4 className="text-xs font-medium">{t("addModels")}</h4>
            <div className="flex items-center gap-2">
              <Input
                value={manualInput}
                onChange={(event) => setManualInput(event.target.value)}
                placeholder={t("modelsPlaceholder")}
                className="h-8 flex-1 text-xs"
                disabled={saving}
              />
              <Button
                type="button"
                size="sm"
                className="h-8 shrink-0 gap-1 text-xs"
                disabled={saving || manualInput.trim() === ""}
                onClick={() => void submitManual()}
              >
                <Plus className="size-3.5 stroke-1" />
                {t("addModels")}
              </Button>
            </div>
          </div>
        </div>
        <DialogFooter className="shrink-0 px-4 py-3">
          {saving ? <SpinnerLabel>{t("saving")}</SpinnerLabel> : null}
          <Button type="button" variant="ghost" disabled={saving} onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
        </DialogFooter>
      </UpstreamModelsDialogContent>
    </Dialog>
  );
}
