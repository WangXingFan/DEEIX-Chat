"use client";

import { Plus, RefreshCw, Search } from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import type { UserRemoteModelDTO, UserUpstreamDTO, UserUpstreamModelDTO } from "@/shared/api/upstreams-types";
import { UpstreamModelsDialogContent } from "@/shared/components/upstream-models-dialog-content";

type ModelRow = {
  name: string;
  added: boolean;
  selected: boolean;
  available: boolean;
};

function boundModelAvailable(model: UserUpstreamModelDTO): boolean {
  return model.routeStatus === "active" && model.upstreamModelStatus === "active" && !model.circuitOpen;
}

export function UpstreamsModelsDialog({
  open,
  onOpenChange,
  upstream,
  addedModels,
  saving,
  onDiscover,
  onSyncModels,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  upstream: UserUpstreamDTO | null;
  addedModels: UserUpstreamModelDTO[];
  saving: boolean;
  onDiscover: (upstreamID: number) => Promise<UserRemoteModelDTO[]>;
  onSyncModels: (upstreamID: number, modelNames: string[]) => Promise<boolean>;
}) {
  const t = useTranslations("settings.upstreamsPage");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const [remoteModels, setRemoteModels] = React.useState<UserRemoteModelDTO[] | null>(null);
  const [remoteLoading, setRemoteLoading] = React.useState(false);
  const [remoteError, setRemoteError] = React.useState("");
  const [query, setQuery] = React.useState("");
  const [manualInput, setManualInput] = React.useState("");
  // 只保存用户的选择意图；刷新远端列表和同步失败后的重新加载都不丢弃待同步选择。
  const [selection, setSelection] = React.useState<Map<string, boolean>>(() => new Map());
  const remoteRequestSeqRef = React.useRef(0);
  const upstreamID = upstream?.id ?? 0;

  const loadRemoteModels = React.useCallback(async (id: number) => {
    const requestSeq = ++remoteRequestSeqRef.current;
    setRemoteLoading(true);
    setRemoteError("");
    try {
      const models = await onDiscover(id);
      if (requestSeq === remoteRequestSeqRef.current) {
        setRemoteModels(models);
      }
    } catch (error) {
      if (requestSeq === remoteRequestSeqRef.current) {
        setRemoteModels(null);
        setRemoteError(resolveErrorMessage(error));
      }
    } finally {
      if (requestSeq === remoteRequestSeqRef.current) {
        setRemoteLoading(false);
      }
    }
  }, [onDiscover, resolveErrorMessage]);

  React.useEffect(() => {
    if (!open) {
      return;
    }
    setQuery("");
    setManualInput("");
    setSelection(new Map());
  }, [open, upstreamID]);

  React.useEffect(() => {
    if (!open) {
      return;
    }
    setRemoteModels(null);
    if (upstreamID > 0) {
      void loadRemoteModels(upstreamID);
    }
    return () => {
      remoteRequestSeqRef.current += 1;
    };
  }, [loadRemoteModels, open, upstreamID]);

  const rows = React.useMemo<ModelRow[]>(() => {
    const boundNames = new Set<string>();
    const availableNames = new Set<string>();
    for (const model of addedModels) {
      boundNames.add(model.upstreamModelName);
      if (boundModelAvailable(model)) {
        availableNames.add(model.upstreamModelName);
      }
    }
    // 保留远端未返回的已绑定模型，以及手动输入或等待重试的模型。
    const names = new Set([
      ...(remoteModels ?? []).map((model) => model.upstreamModelName),
      ...boundNames,
      ...selection.keys(),
    ]);
    return Array.from(names, (name) => ({
      name,
      added: boundNames.has(name),
      selected: selection.get(name) ?? boundNames.has(name),
      available: availableNames.has(name),
    })).sort((left, right) => left.name.localeCompare(right.name));
  }, [addedModels, remoteModels, selection]);

  const filteredRows = React.useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (!keyword) {
      return rows;
    }
    return rows.filter((row) => row.name.toLowerCase().includes(keyword));
  }, [query, rows]);

  function toggleModel(row: ModelRow, checked: boolean) {
    if (saving) {
      return;
    }
    setSelection((previous) => new Map(previous).set(row.name, checked));
  }

  function submitManual() {
    const names = manualInput.split(",").map((name) => name.trim()).filter(Boolean);
    if (saving || upstreamID === 0 || names.length === 0) {
      return;
    }
    setSelection((previous) => {
      const next = new Map(previous);
      for (const name of names) {
        next.set(name, true);
      }
      return next;
    });
    setManualInput("");
  }

  async function syncModels() {
    if (saving || upstreamID === 0) {
      return;
    }
    if (await onSyncModels(upstreamID, rows.filter((row) => row.selected).map((row) => row.name))) {
      setSelection(new Map());
    }
  }

  const selectedCount = rows.filter((row) => row.selected).length;
  const additions = rows.filter((row) => row.selected && !row.added).length;
  const removals = rows.filter((row) => !row.selected && row.added).length;
  const hasChanges = additions + removals > 0;

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => !saving && onOpenChange(nextOpen)}>
      <UpstreamModelsDialogContent>
        <DialogHeader className="shrink-0 px-4 py-4">
          <DialogTitle>{t("manageModelsTitle", { name: upstream?.name ?? "" })}</DialogTitle>
          <DialogDescription>{t("discoverDescription")}</DialogDescription>
        </DialogHeader>
        <div className="flex min-h-0 flex-1 flex-col gap-2 px-4 pb-2">
          <div className="flex items-center gap-2">
            <div className="relative flex-1">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 stroke-1 text-muted-foreground" />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t("searchModels")}
                className="h-8 pl-8 text-xs"
                disabled={remoteLoading}
              />
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="h-8 shrink-0 gap-1 text-xs"
              disabled={saving || remoteLoading || upstreamID === 0}
              onClick={() => void loadRemoteModels(upstreamID)}
            >
              <RefreshCw className={remoteLoading ? "size-3.5 stroke-1 animate-spin" : "size-3.5 stroke-1"} />
              {t("discover")}
            </Button>
          </div>
          <div className="min-h-0 flex-1 space-y-1 overflow-y-auto">
            {remoteLoading && rows.length === 0 ? (
              <div className="px-1 py-3 text-[11px] text-muted-foreground">{t("loading")}</div>
            ) : remoteError && rows.length === 0 ? (
              <div className="space-y-2 px-1 py-3">
                <p className="text-[11px] leading-4 text-muted-foreground">{remoteError}</p>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="h-7 text-xs"
                  disabled={saving || remoteLoading}
                  onClick={() => void loadRemoteModels(upstreamID)}
                >
                  {t("retry")}
                </Button>
              </div>
            ) : rows.length === 0 ? (
              <p className="px-1 py-3 text-[11px] leading-4 text-muted-foreground">{t("noRemoteModels")}</p>
            ) : filteredRows.length === 0 ? (
              <p className="px-1 py-3 text-[11px] leading-4 text-muted-foreground">{t("noMatchingModels")}</p>
            ) : (
              filteredRows.map((row) => (
                <label
                  key={row.name}
                  className="flex cursor-pointer items-center gap-2 rounded-md px-1 py-1.5 text-xs hover:bg-muted/60"
                >
                  <Checkbox
                    checked={row.selected}
                    disabled={saving}
                    onCheckedChange={(value) => toggleModel(row, value === true)}
                  />
                  <span className="min-w-0 flex-1 truncate" title={row.name}>
                    {row.name}
                  </span>
                  {row.selected !== row.added ? (
                    <Badge variant="outline" className="shrink-0 text-[10px]">
                      {row.selected ? t("pendingAdd") : t("pendingRemove")}
                    </Badge>
                  ) : row.added ? (
                    <Badge variant={row.available ? "secondary" : "outline"} className="shrink-0 text-[10px]">
                      {row.available ? t("available") : t("unavailable")}
                    </Badge>
                  ) : null}
                </label>
              ))
            )}
          </div>
          <div className="flex items-center gap-2 border-t border-border/60 pt-2">
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
              onClick={submitManual}
            >
              <Plus className="size-3.5 stroke-1" />
              {t("selectModels")}
            </Button>
          </div>
        </div>
        <DialogFooter className="shrink-0 px-4 py-3">
          <span className="mr-auto text-[11px] text-muted-foreground" aria-live="polite">
            {saving ? t("syncing") : hasChanges
              ? t("pendingChanges", { additions, removals })
              : t("selectedModels", { count: selectedCount })}
          </span>
          <Button type="button" variant="ghost" disabled={saving} onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <Button type="button" disabled={saving || upstreamID === 0 || !hasChanges} onClick={() => void syncModels()}>
            <RefreshCw className={saving ? "size-3.5 stroke-1 animate-spin" : "size-3.5 stroke-1"} />
            {saving ? t("syncing") : t("sync")}
          </Button>
        </DialogFooter>
      </UpstreamModelsDialogContent>
    </Dialog>
  );
}
