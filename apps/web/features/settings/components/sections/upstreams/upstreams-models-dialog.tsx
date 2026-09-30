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

// 勾选状态直接由“是否已绑定”推导：勾上即已添加，取消勾选即移除，不再维护本地待提交集合。
type ModelRow = {
  name: string;
  added: boolean;
  routeID: number;
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
  const [query, setQuery] = React.useState("");
  const [manualInput, setManualInput] = React.useState("");
  const [togglingName, setTogglingName] = React.useState("");
  const upstreamID = upstream?.id ?? 0;

  const loadRemoteModels = React.useCallback(async (id: number) => {
    setRemoteLoading(true);
    setRemoteError("");
    try {
      setRemoteModels(await onDiscover(id));
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
    setQuery("");
    setManualInput("");
    setTogglingName("");
    setRemoteModels(null);
    if (upstreamID > 0) {
      void loadRemoteModels(upstreamID);
    }
  }, [loadRemoteModels, open, upstreamID]);

  const rows = React.useMemo<ModelRow[]>(() => {
    const boundByName = new Map(addedModels.map((model) => [model.upstreamModelName, model]));
    const merged: ModelRow[] = [];
    const seen = new Set<string>();
    for (const remote of remoteModels ?? []) {
      const bound = boundByName.get(remote.upstreamModelName);
      seen.add(remote.upstreamModelName);
      merged.push({
        name: remote.upstreamModelName,
        added: bound !== undefined,
        routeID: bound?.routeID ?? 0,
        available: bound ? boundModelAvailable(bound) : false,
      });
    }
    // 远端发现不返回、但已经添加过的模型同样要能取消勾选。
    for (const model of addedModels) {
      if (seen.has(model.upstreamModelName)) {
        continue;
      }
      merged.push({
        name: model.upstreamModelName,
        added: true,
        routeID: model.routeID,
        available: boundModelAvailable(model),
      });
    }
    return merged.sort((left, right) => left.name.localeCompare(right.name));
  }, [addedModels, remoteModels]);

  const filteredRows = React.useMemo(() => {
    const keyword = query.trim().toLowerCase();
    if (!keyword) {
      return rows;
    }
    return rows.filter((row) => row.name.toLowerCase().includes(keyword));
  }, [query, rows]);

  async function toggleModel(row: ModelRow, checked: boolean) {
    if (upstreamID === 0 || togglingName !== "") {
      return;
    }
    setTogglingName(row.name);
    try {
      if (checked) {
        await onAddModels(upstreamID, [row.name]);
      } else if (row.routeID > 0) {
        await onRemoveModel(upstreamID, row.routeID);
      }
    } finally {
      setTogglingName("");
    }
  }

  async function submitManual() {
    const names = manualInput.split(",").map((name) => name.trim()).filter(Boolean);
    if (upstreamID === 0 || names.length === 0) {
      return;
    }
    const existing = new Set(rows.filter((row) => row.added).map((row) => row.name));
    const pending = names.filter((name) => !existing.has(name));
    if (pending.length === 0) {
      setManualInput("");
      return;
    }
    if (await onAddModels(upstreamID, pending)) {
      setManualInput("");
    }
  }

  const addedCount = rows.filter((row) => row.added).length;

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
                    checked={row.added}
                    disabled={saving || togglingName !== ""}
                    onCheckedChange={(value) => void toggleModel(row, value === true)}
                  />
                  <span className="min-w-0 flex-1 truncate" title={row.name}>
                    {row.name}
                  </span>
                  {row.added ? (
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
              onClick={() => void submitManual()}
            >
              <Plus className="size-3.5 stroke-1" />
              {t("addModels")}
            </Button>
          </div>
        </div>
        <DialogFooter className="shrink-0 px-4 py-3">
          <span className="mr-auto text-[11px] text-muted-foreground">
            {saving ? t("saving") : t("addedModels", { count: addedCount })}
          </span>
          <Button type="button" variant="ghost" disabled={saving} onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
        </DialogFooter>
      </UpstreamModelsDialogContent>
    </Dialog>
  );
}
