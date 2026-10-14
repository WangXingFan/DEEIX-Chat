"use client";

import * as React from "react";
import { Archive, SquareMousePointer, Trash, X } from "lucide-react";
import { useTranslations } from "next-intl";

import { RecentFilterGroup } from "@/features/recent/components/sections/recent-filter-group";
import {
  RECENT_STARRED_FILTER_OPTIONS,
  RECENT_STATUS_FILTER_OPTIONS,
} from "@/features/recent/utils/recent-display";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type {
  ConversationStarredFilter,
  ConversationStatusFilter,
} from "@/shared/api/conversation-types";

type RecentToolbarProps = {
  isSelectionMode: boolean;
  selectedCount: number;
  pageSelectionState: boolean | "indeterminate";
  statusFilter: ConversationStatusFilter;
  starredFilter: ConversationStarredFilter;
  allSelectedArchived: boolean;
  onToggleSelectionMode: (checked: boolean | "indeterminate") => void;
  onEnterSelectionMode: () => void;
  onExitSelectionMode: () => void;
  onArchiveSelected: () => void | Promise<void>;
  onRequestDeleteSelected: () => void;
  onStatusFilterChange: (value: ConversationStatusFilter) => void;
  onStarredFilterChange: (value: ConversationStarredFilter) => void;
};

function ToolbarActionTooltip({
  children,
  label,
}: {
  children: React.ReactNode;
  label: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">{children}</span>
      </TooltipTrigger>
      <TooltipContent side="top" sideOffset={6}>
        {label}
      </TooltipContent>
    </Tooltip>
  );
}

export function RecentToolbar({
  isSelectionMode,
  selectedCount,
  pageSelectionState,
  statusFilter,
  starredFilter,
  allSelectedArchived,
  onToggleSelectionMode,
  onEnterSelectionMode,
  onExitSelectionMode,
  onArchiveSelected,
  onRequestDeleteSelected,
  onStatusFilterChange,
  onStarredFilterChange,
}: RecentToolbarProps) {
  const t = useTranslations("recent");
  const statusOptions = React.useMemo(
    () => RECENT_STATUS_FILTER_OPTIONS.map((item) => ({ ...item, label: t(item.value) })),
    [t],
  );
  const starredOptions = React.useMemo(
    () => RECENT_STARRED_FILTER_OPTIONS.map((item) => ({ ...item, label: t(item.value) })),
    [t],
  );
  const filterGroups = (
    <div className="flex w-full min-w-0 items-center gap-1 overflow-x-auto whitespace-nowrap pb-0.5 [scrollbar-width:none] [-ms-overflow-style:none] md:w-auto md:justify-end md:pb-0 [&::-webkit-scrollbar]:hidden">
      <RecentFilterGroup
        label={t("status")}
        value={statusFilter}
        options={statusOptions}
        onChange={onStatusFilterChange}
      />

      <RecentFilterGroup
        label={t("star")}
        value={starredFilter}
        options={starredOptions}
        onChange={onStarredFilterChange}
      />
    </div>
  );

  return (
    <div className="group mt-6 flex w-full items-start md:items-center">
      <div className="hidden w-13 shrink-0 items-center justify-center md:flex">
        <Checkbox
          checked={pageSelectionState}
          aria-label={isSelectionMode ? t("exitSelection") : t("enterSelection")}
          className={cn(
            "transition-opacity duration-150",
            isSelectionMode ? "opacity-100" : "opacity-0 group-hover:opacity-100",
          )}
          onCheckedChange={onToggleSelectionMode}
        />
      </div>

      <div className="flex w-full min-w-0 flex-col gap-2 px-1 text-sm md:w-[calc(100%-3.25rem)] md:flex-row md:items-center md:justify-between md:gap-2 md:px-3">
        {isSelectionMode ? (
          <>
            <div className="flex w-full min-w-0 items-center gap-4 overflow-x-auto whitespace-nowrap text-foreground/70 [scrollbar-width:none] [-ms-overflow-style:none] md:w-auto md:shrink-0 md:overflow-visible [&::-webkit-scrollbar]:hidden">
              <span>{t("selectedCount", { count: selectedCount })}</span>
              <ToolbarActionTooltip
                label={allSelectedArchived ? t("unarchiveSelected") : t("archiveSelected")}
              >
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className={cn(
                    "transition-colors",
                    selectedCount > 0
                      ? "text-foreground/60 hover:bg-accent hover:text-foreground"
                      : "text-muted-foreground/50",
                  )}
                  onClick={() => void onArchiveSelected()}
                  disabled={selectedCount === 0}
                  aria-label={allSelectedArchived ? t("unarchiveSelected") : t("archiveSelected")}
                >
                  <Archive className="size-4.5" strokeWidth={1} />
                </Button>
              </ToolbarActionTooltip>
              <ToolbarActionTooltip label={t("deleteSelected")}>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className={cn(
                    "transition-colors",
                    selectedCount > 0
                      ? "text-foreground/60 hover:bg-accent hover:text-foreground"
                      : "text-muted-foreground/50",
                  )}
                  onClick={onRequestDeleteSelected}
                  disabled={selectedCount === 0}
                  aria-label={t("deleteSelected")}
                >
                  <Trash className="size-4.5" strokeWidth={1} />
                </Button>
              </ToolbarActionTooltip>
              <ToolbarActionTooltip label={t("exitSelection")}>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="text-foreground/60 transition-colors hover:bg-accent hover:text-foreground"
                  onClick={onExitSelectionMode}
                  aria-label={t("exitSelection")}
                >
                  <X className="size-4.5" strokeWidth={1} />
                </Button>
              </ToolbarActionTooltip>
            </div>

            {filterGroups}
          </>
        ) : (
          <>
            <div className="flex w-full min-w-0 items-center justify-start gap-4 text-foreground/60 md:w-auto md:shrink-0">
              <span className="min-w-0 truncate md:hidden">{t("allConversations")}</span>
              <span className="hidden md:inline">{t("allConversationsDescription")}</span>
              <ToolbarActionTooltip label={t("enterSelection")}>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="transition-colors hover:bg-accent hover:text-foreground"
                  onClick={onEnterSelectionMode}
                  aria-label={t("enterSelection")}
                >
                  <SquareMousePointer className="size-4" strokeWidth={1.4} />
                </Button>
              </ToolbarActionTooltip>
            </div>

            {filterGroups}
          </>
        )}
      </div>
    </div>
  );
}
