"use client";

import { MoreHorizontal, Pencil, Plus, RefreshCw, Settings2, Trash2 } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import * as React from "react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Table,
  TableBody,
  TableCell,
  TableEmptyRow,
  TableHead,
  TableHeader,
  TableLoadingRow,
  TableRow,
} from "@/components/ui/table";
import { TablePagination, TableToolbar } from "@/components/ui/table-tools";
import { UPSTREAM_COMPATIBLE_OPTIONS } from "@/entities/model";
import { useSettingsUpstreams, USER_UPSTREAM_SORT_OPTIONS } from "@/features/settings/hooks/use-settings-upstreams";
import type { UserUpstreamDTO } from "@/shared/api/upstreams-types";
import { SettingsPage, SettingsSection } from "@/shared/components/settings-layout";
import { UpstreamsModelsDialog } from "./upstreams-models-dialog";
import { UpstreamsSheet } from "./upstreams-sheet";

const PAGE_SIZE_OPTIONS = [10, 20, 50, 100] as const;

function formatDateTime(value: string, locale: string): string {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function SettingsUpstreams() {
  const t = useTranslations("settings.upstreamsPage");
  const locale = useLocale();
  const upstreams = useSettingsUpstreams();
  const [sheetTarget, setSheetTarget] = React.useState<UserUpstreamDTO | null>(null);
  const [sheetOpen, setSheetOpen] = React.useState(false);
  const [modelsTarget, setModelsTarget] = React.useState<UserUpstreamDTO | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<UserUpstreamDTO | null>(null);

  const resolveCompatibleLabel = React.useCallback(
    (compatible: string) =>
      UPSTREAM_COMPATIBLE_OPTIONS.find((option) => option.value === compatible)?.label ?? (compatible || "-"),
    [],
  );

  function openCreate() {
    setSheetTarget(null);
    setSheetOpen(true);
  }

  function openEdit(upstream: UserUpstreamDTO) {
    setSheetTarget(upstream);
    setSheetOpen(true);
  }

  const initialLoading = upstreams.loading && upstreams.upstreams.length === 0;

  return (
    <SettingsPage>
      <SettingsSection title={t("title")}>
        <p className="text-sm text-muted-foreground">{t("description")}</p>

        <TableToolbar
          query={upstreams.query}
          onQueryChange={upstreams.setQuery}
          queryPlaceholder={t("table.searchPlaceholder")}
          filters={[
            {
              key: "status",
              label: t("table.status"),
              value: upstreams.statusFilter,
              onValueChange: upstreams.setStatusFilter,
              options: [
                { label: t("allStatus"), value: "" },
                { label: t("status.active"), value: "active" },
                { label: t("status.inactive"), value: "inactive" },
              ],
            },
            {
              key: "compatible",
              label: t("table.compatible"),
              value: upstreams.compatibleFilter,
              onValueChange: upstreams.setCompatibleFilter,
              options: [
                { label: t("allCompatible"), value: "" },
                ...UPSTREAM_COMPATIBLE_OPTIONS.map((option) => ({
                  label: option.value === "custom" ? t("compatibleCustom") : option.label,
                  value: option.value,
                })),
              ],
            },
          ]}
          sort={{
            value: upstreams.sortValue,
            onValueChange: (value) => {
              const option = USER_UPSTREAM_SORT_OPTIONS.find((item) => item.value === value);
              if (option) {
                upstreams.setSortValue(option.value);
              }
            },
            options: USER_UPSTREAM_SORT_OPTIONS.map((option) => ({ label: t(option.labelKey), value: option.value })),
          }}
          loading={upstreams.loading}
          onRefresh={() => void upstreams.reload()}
        >
          <Button type="button" size="sm" className="h-7 gap-1 text-xs" onClick={openCreate} disabled={upstreams.loading}>
            <Plus className="size-3.5 stroke-1" />
            {t("addTitle")}
          </Button>
        </TableToolbar>

        {upstreams.loadError ? (
          <div className="flex items-center justify-between gap-3 rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2">
            <span className="min-w-0 truncate text-xs text-destructive">{t("loadFailed")}</span>
            <Button type="button" size="sm" variant="outline" className="h-7 gap-1 text-xs" onClick={() => void upstreams.reload()}>
              <RefreshCw className="size-3.5 stroke-1" />
              {t("retry")}
            </Button>
          </div>
        ) : null}

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("table.name")}</TableHead>
              <TableHead>{t("table.url")}</TableHead>
              <TableHead>{t("table.compatible")}</TableHead>
              <TableHead>{t("table.models")}</TableHead>
              <TableHead className="text-center">{t("table.status")}</TableHead>
              <TableHead>{t("table.updatedAt")}</TableHead>
              <TableHead className="w-[56px]" stickyEnd />
            </TableRow>
          </TableHeader>
          <TableBody>
            {initialLoading ? <TableLoadingRow colSpan={7} /> : null}
            {!upstreams.loading && upstreams.upstreams.length === 0 ? (
              <TableEmptyRow colSpan={7}>{t("empty")}</TableEmptyRow>
            ) : null}
            {upstreams.upstreams.map((upstream) => (
              <TableRow key={upstream.id}>
                <TableCell className="whitespace-nowrap">
                  <div className="max-w-[18rem] truncate font-medium">{upstream.name}</div>
                </TableCell>
                <TableCell>
                  <div className="max-w-[16rem] truncate text-xs text-muted-foreground" title={upstream.baseURL}>
                    {upstream.baseURL}
                  </div>
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  <Badge variant="secondary">
                    {upstream.compatible === "custom" ? t("compatibleCustom") : resolveCompatibleLabel(upstream.compatible)}
                  </Badge>
                </TableCell>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                  {t("modelCountSummary", { active: upstream.activeModelsCount, total: upstream.modelsCount })}
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  <div className="flex items-center justify-center gap-1.5">
                    <Badge variant={upstream.status === "active" ? "secondary" : "outline"}>
                      {upstream.status === "active" ? t("status.active") : t("status.inactive")}
                    </Badge>
                    {upstream.circuitOpen ? <Badge variant="destructive">{t("status.circuitOpen")}</Badge> : null}
                  </div>
                </TableCell>
                <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                  {formatDateTime(upstream.updatedAt, locale)}
                </TableCell>
                <TableCell className="w-[56px] whitespace-nowrap" stickyEnd>
                  <div className="flex items-center justify-end">
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button type="button" variant="ghost" size="icon-xs" className="text-muted-foreground shadow-none">
                          <MoreHorizontal className="size-3.5 stroke-1" />
                          <span className="sr-only">{t("table.actions")}</span>
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onSelect={() => openEdit(upstream)}>
                          <Pencil className="size-3.5 stroke-1" />
                          {t("actions.edit")}
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => setModelsTarget(upstream)}>
                          <Settings2 className="size-3.5 stroke-1" />
                          {t("actions.manageModels")}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem className="text-destructive" onSelect={() => setDeleteTarget(upstream)}>
                          <Trash2 className="size-3.5 stroke-1" />
                          {t("actions.delete")}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>

        <TablePagination
          total={upstreams.total}
          page={upstreams.page}
          pageCount={upstreams.pageCount}
          pageSize={upstreams.pageSize}
          pageSizeOptions={PAGE_SIZE_OPTIONS}
          onPageChange={upstreams.setPage}
          onPageSizeChange={upstreams.setPageSize}
          loading={upstreams.loading}
        />
      </SettingsSection>

      <UpstreamsSheet
        open={sheetOpen}
        onOpenChange={setSheetOpen}
        upstream={sheetTarget}
        saving={upstreams.saving}
        onCreate={upstreams.addUpstream}
        onUpdate={upstreams.editUpstream}
      />

      <UpstreamsModelsDialog
        open={modelsTarget !== null}
        onOpenChange={(open) => !open && setModelsTarget(null)}
        upstream={modelsTarget}
        addedModels={modelsTarget ? upstreams.addedModels[modelsTarget.id] ?? [] : []}
        saving={upstreams.saving}
        onDiscover={upstreams.discover}
        onSyncModels={upstreams.syncModels}
      />

      <AlertDialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent size="compact">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("deleteTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("confirmDelete")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                if (deleteTarget) {
                  void upstreams.remove(deleteTarget.id);
                }
                setDeleteTarget(null);
              }}
            >
              {t("delete")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsPage>
  );
}
