"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";

import {
  isArchivedConversation,
  mergeUniqueByPublicID,
  normalizeConversationSearchText,
  removeByPublicID,
  sortByUpdatedAtDesc,
  upsertByPublicID,
  useSidebarConversationField,
} from "@/entities/conversation";
import { useChatSession } from "@/features/chat";
import type { RecentDeleteTarget, RecentRowState } from "@/features/recent/types/recent";
import { RECENT_PAGE_SIZE } from "@/features/recent/utils/recent-display";
import { useSettingsChatPreferences } from "@/features/settings";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import { listConversations } from "@/shared/api/conversation";
import type {
  ConversationDTO,
  ConversationProjectFilter,
  ConversationStarredFilter,
  ConversationStatusFilter,
} from "@/shared/api/conversation-types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { useDebouncedValue } from "@/shared/hooks/use-debounced-value";
import { useLoadMoreSentinel } from "@/shared/hooks/use-load-more-sentinel";
import { runBulkActionInChunks } from "@/shared/lib/bulk-action";

const RECENT_SEARCH_DEBOUNCE_MS = 250;

function conversationMatchesRecentFilters(
  item: ConversationDTO,
  statusFilter: ConversationStatusFilter,
  starredFilter: ConversationStarredFilter,
  projectFilter: ConversationProjectFilter,
): boolean {
  if (statusFilter === "archived" && !isArchivedConversation(item)) {
    return false;
  }
  if (statusFilter === "active" && isArchivedConversation(item)) {
    return false;
  }
  if (starredFilter === "starred" && !item.isStarred) {
    return false;
  }
  if (starredFilter === "unstarred" && item.isStarred) {
    return false;
  }
  if (projectFilter === "unassigned") {
    return !item.projectID;
  }
  if (projectFilter !== "all") {
    return item.projectID === projectFilter;
  }
  return true;
}

export function useRecentPage() {
  const t = useTranslations("recent");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const router = useRouter();
  const { requestNewConversation } = useChatSession();
  const renameByPublicID = useSidebarConversationField("renameByPublicID");
  const regenerateTitleByPublicID = useSidebarConversationField("regenerateTitleByPublicID");
  const updateLabelsByPublicID = useSidebarConversationField("updateLabelsByPublicID");
  const setStarByPublicID = useSidebarConversationField("setStarByPublicID");
  const archiveByPublicID = useSidebarConversationField("archiveByPublicID");
  const deleteByPublicID = useSidebarConversationField("deleteByPublicID");
  const projects = useSidebarConversationField("projects");
  const setProjectByPublicID = useSidebarConversationField("setProjectByPublicID");
  const batchSetProjectByPublicIDs = useSidebarConversationField("batchSetProjectByPublicIDs");
  const lastChange = useSidebarConversationField("lastChange");
  const [items, setItems] = React.useState<ConversationDTO[]>([]);
  const [loadingInitial, setLoadingInitial] = React.useState(true);
  const [loadingMore, setLoadingMore] = React.useState(false);
  const [hasMore, setHasMore] = React.useState(true);
  const [loadMoreFailed, setLoadMoreFailed] = React.useState(false);
  const [statusFilter, setStatusFilter] = React.useState<ConversationStatusFilter>("all");
  const [starredFilter, setStarredFilter] = React.useState<ConversationStarredFilter>("all");
  const searchParams = useSearchParams();
  const [projectFilter, setProjectFilter] = React.useState<ConversationProjectFilter>(() => searchParams.get("project") || "all");
  const [query, setQuery] = React.useState("");
  const debouncedQuery = useDebouncedValue(query, RECENT_SEARCH_DEBOUNCE_MS);
  const [selectionMode, setSelectionMode] = React.useState(false);
  const [hoveredConversationID, setHoveredConversationID] = React.useState<string | null>(null);
  const [selectedConversationIDs, setSelectedConversationIDs] = React.useState<string[]>([]);
  const [renameTarget, setRenameTarget] = React.useState<ConversationDTO | null>(null);
  const [renameValue, setRenameValue] = React.useState("");
  const [renamingAutomatically, setRenamingAutomatically] = React.useState(false);
  const [labelsTarget, setLabelsTarget] = React.useState<ConversationDTO | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<RecentDeleteTarget>(null);
  const [deleteFiles, setDeleteFiles] = React.useState(false);
  const { deleteFilesByDefault } = useSettingsChatPreferences();
  const [movingSelectedToProject, setMovingSelectedToProject] = React.useState(false);
  const pageRef = React.useRef(1);
  const requestVersionRef = React.useRef(0);
  const loadingMoreRef = React.useRef(false);
  const loadMoreFailedRef = React.useRef(false);
  const isSelectionMode = selectionMode || selectedConversationIDs.length > 0;

  React.useEffect(() => {
    setProjectFilter(searchParams.get("project") || "all");
  }, [searchParams]);

  React.useEffect(() => {
    loadingMoreRef.current = loadingMore;
  }, [loadingMore]);

  React.useEffect(() => {
    loadMoreFailedRef.current = loadMoreFailed;
  }, [loadMoreFailed]);

  const normalizedQuery = normalizeConversationSearchText(debouncedQuery);
  const filteredItems = items;

  const lastAppliedChangeSequenceRef = React.useRef(0);

  React.useEffect(() => {
    if (!lastChange || lastChange.sequence <= lastAppliedChangeSequenceRef.current) {
      return;
    }
    lastAppliedChangeSequenceRef.current = lastChange.sequence;

    if (lastChange.type === "remove") {
      setItems((current) => removeByPublicID(current, lastChange.publicID));
      setSelectedConversationIDs((current) => current.filter((item) => item !== lastChange.publicID));
      return;
    }

    if (lastChange.type === "patch" && lastChange.patch) {
      setItems((current) =>
        current
          .map((item) => (item.publicID === lastChange.publicID ? { ...item, ...lastChange.patch } : item))
          .filter((item) => conversationMatchesRecentFilters(item, statusFilter, starredFilter, projectFilter)),
      );
      return;
    }

    const changedItem = lastChange.item;
    if (!changedItem) {
      return;
    }

    if (!conversationMatchesRecentFilters(changedItem, statusFilter, starredFilter, projectFilter)) {
      setItems((current) => removeByPublicID(current, lastChange.publicID));
      setSelectedConversationIDs((current) => current.filter((item) => item !== lastChange.publicID));
      return;
    }

    if (normalizedQuery && !items.some((item) => item.publicID === lastChange.publicID)) {
      return;
    }

    setItems((current) => upsertByPublicID(current, changedItem));
  }, [items, lastChange, normalizedQuery, projectFilter, starredFilter, statusFilter]);

  const loadPage = React.useCallback(
    async (page: number, options?: { replace?: boolean; version?: number }) => {
      const requestVersion = options?.version ?? requestVersionRef.current;
      const token = await resolveAccessToken();
      if (!token) {
        if (requestVersion === requestVersionRef.current) {
          setItems([]);
          setHasMore(false);
        }
        return;
      }

      const data = await listConversations(token, {
        page,
        pageSize: RECENT_PAGE_SIZE,
        status: statusFilter,
        starred: starredFilter,
        project: projectFilter,
        query: normalizedQuery,
      });
      if (requestVersion !== requestVersionRef.current) {
        return;
      }

      const nextResults = data.results ?? [];
      setItems((current) => (
        options?.replace ? sortByUpdatedAtDesc(nextResults) : mergeUniqueByPublicID(current, nextResults)
      ));

      const loaded = data.results?.length ?? 0;
      const total = data.total ?? 0;
      const mergedCount = (page - 1) * RECENT_PAGE_SIZE + loaded;
      setHasMore(loaded === RECENT_PAGE_SIZE && mergedCount < total);
      setLoadMoreFailed(false);
      loadMoreFailedRef.current = false;
      pageRef.current = page;
    },
    [normalizedQuery, projectFilter, starredFilter, statusFilter],
  );

  React.useEffect(() => {
    requestVersionRef.current += 1;
    const version = requestVersionRef.current;

    setLoadingInitial(true);
    setItems([]);
    setHasMore(true);
    setLoadMoreFailed(false);
    loadMoreFailedRef.current = false;
    setSelectionMode(false);
    setSelectedConversationIDs([]);
    setHoveredConversationID(null);
    pageRef.current = 1;

    let cancelled = false;
    (async () => {
      try {
        await loadPage(1, { replace: true, version });
      } finally {
        if (!cancelled && version === requestVersionRef.current) {
          setLoadingInitial(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [loadPage]);

  const loadMore = React.useCallback(async () => {
    if (loadingInitial || loadingMoreRef.current || !hasMore || loadMoreFailedRef.current) {
      return;
    }

    loadingMoreRef.current = true;
    setLoadingMore(true);
    try {
      await loadPage(pageRef.current + 1, { version: requestVersionRef.current });
    } catch (error) {
      loadMoreFailedRef.current = true;
      setLoadMoreFailed(true);
      const description = resolveErrorMessage(error, t("loadMoreFailed"));
      toast.error(t("loadMoreFailed"), {
        id: "recent-load-more-error",
        description,
      });
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }, [hasMore, loadPage, loadingInitial, resolveErrorMessage, t]);

  const loadMoreRef = useLoadMoreSentinel<HTMLDivElement>({
    enabled: hasMore && !loadingInitial && !loadingMore && !loadMoreFailed,
    rootMargin: "160px",
    onLoadMore: loadMore,
  });

  const retryLoadMore = React.useCallback(async () => {
    setLoadMoreFailed(false);
    loadMoreFailedRef.current = false;
    await loadMore();
  }, [loadMore]);

  const onCreateConversation = React.useCallback(() => {
    const currentProjectID = projectFilter !== "all" && projectFilter !== "unassigned" ? projectFilter : "";
    requestNewConversation({ projectID: currentProjectID });
    router.push(currentProjectID ? `/chat?project_id=${encodeURIComponent(currentProjectID)}` : "/chat");
  }, [projectFilter, requestNewConversation, router]);

  const onProjectFilterChange = React.useCallback(
    (value: ConversationProjectFilter) => {
      setProjectFilter(value);
      const href = value === "all" ? "/recent" : `/recent?project=${encodeURIComponent(value)}`;
      router.replace(href);
    },
    [router],
  );

  const onToggleSelected = React.useCallback((publicID: string) => {
    setSelectionMode(true);
    setSelectedConversationIDs((current) =>
      current.includes(publicID) ? current.filter((item) => item !== publicID) : [...current, publicID],
    );
  }, []);

  const onToggleStar = React.useCallback(
    async (publicID: string, nextStarred: boolean) => {
      const updated = await setStarByPublicID(publicID, nextStarred);
      if (!updated) {
        return;
      }
      setItems((current) => (
        conversationMatchesRecentFilters(updated, statusFilter, starredFilter, projectFilter)
          ? upsertByPublicID(current, updated)
          : removeByPublicID(current, publicID)
      ));
    },
    [projectFilter, setStarByPublicID, starredFilter, statusFilter],
  );

  const onRename = React.useCallback((item: ConversationDTO) => {
    setRenameTarget(item);
    setRenameValue(item.title || t("untitled"));
  }, [t]);

  const onManageLabels = React.useCallback((item: ConversationDTO) => {
    setLabelsTarget(item);
  }, []);

  const onUpdateLabels = React.useCallback(async (labels: string[]) => {
    if (!labelsTarget) {
      throw new Error("conversation label target is unavailable");
    }
    const updated = await updateLabelsByPublicID(labelsTarget.publicID, labels);
    if (!updated) {
      throw new Error("conversation labels were not updated");
    }
    setItems((current) => upsertByPublicID(current, updated));
    setLabelsTarget(updated);
  }, [labelsTarget, updateLabelsByPublicID]);

  const onArchive = React.useCallback(
    async (publicID: string, archived: boolean) => {
      const updated = await archiveByPublicID(publicID, archived);
      setSelectedConversationIDs((current) => current.filter((item) => item !== publicID));
      if (!updated) {
        return;
      }

      setItems((current) => {
        if (!conversationMatchesRecentFilters(updated, statusFilter, starredFilter, projectFilter)) {
          return removeByPublicID(current, publicID);
        }
        return upsertByPublicID(current, updated);
      });
    },
    [archiveByPublicID, projectFilter, starredFilter, statusFilter],
  );

  const onSetProject = React.useCallback(
    async (publicID: string, projectID?: string) => {
      const updated = await setProjectByPublicID(publicID, projectID);
      if (!updated) {
        return;
      }
      setItems((current) =>
        conversationMatchesRecentFilters(updated, statusFilter, starredFilter, projectFilter)
          ? upsertByPublicID(current, updated)
          : removeByPublicID(current, publicID),
      );
      setSelectedConversationIDs((current) => (
        conversationMatchesRecentFilters(updated, statusFilter, starredFilter, projectFilter)
          ? current
          : current.filter((item) => item !== publicID)
      ));
    },
    [projectFilter, setProjectByPublicID, starredFilter, statusFilter],
  );

  const onDelete = React.useCallback((item: ConversationDTO) => {
    setDeleteFiles(deleteFilesByDefault);
    setDeleteTarget({
      ids: [item.publicID],
      label: t("deleteConversationLabel", { title: item.title || t("untitled") }),
    });
  }, [deleteFilesByDefault, t]);

  const onRenameCommit = React.useCallback(async () => {
    if (!renameTarget) {
      return;
    }

    const nextTitle = renameValue.trim();
    if (!nextTitle || nextTitle === renameTarget.title) {
      setRenameTarget(null);
      setRenameValue("");
      return;
    }

    const updated = await renameByPublicID(renameTarget.publicID, nextTitle);
    if (updated) {
      setItems((current) => upsertByPublicID(current, updated));
    }
    setRenameTarget(null);
    setRenameValue("");
  }, [renameByPublicID, renameTarget, renameValue]);

  const onAutoRename = React.useCallback(async () => {
    if (!renameTarget || renamingAutomatically) {
      return;
    }

    setRenamingAutomatically(true);
    try {
      const updated = await regenerateTitleByPublicID(renameTarget.publicID);
      if (updated) {
        setItems((current) => upsertByPublicID(current, updated));
        setRenameTarget(null);
        setRenameValue("");
      }
    } catch (error) {
      toast.error(t("dialogs.autoRenameFailed"), {
        description: resolveErrorMessage(error, t("dialogs.autoRenameFailed")),
      });
    } finally {
      setRenamingAutomatically(false);
    }
  }, [regenerateTitleByPublicID, renameTarget, renamingAutomatically, resolveErrorMessage, t]);

  const confirmDelete = React.useCallback(async () => {
    if (!deleteTarget) {
      return;
    }

    await runBulkActionInChunks({
      chunkSize: 10,
      items: deleteTarget.ids,
      title: t("dialogs.bulk.pending"),
      runChunk: async (ids) => {
        for (const id of ids) {
          await deleteByPublicID(id, deleteFiles ? { deleteFiles: true } : undefined);
        }
      },
    });
    setItems((current) => current.filter((item) => !deleteTarget.ids.includes(item.publicID)));
    setSelectedConversationIDs((current) => current.filter((item) => !deleteTarget.ids.includes(item)));
    if (deleteTarget.ids.length > 1) {
      setSelectionMode(false);
    }
    setDeleteTarget(null);
    setDeleteFiles(false);
  }, [deleteByPublicID, deleteFiles, deleteTarget, t]);

  const closeDeleteDialog = React.useCallback(() => {
    setDeleteTarget(null);
    setDeleteFiles(false);
  }, []);

  const exitSelectionMode = React.useCallback(() => {
    setSelectionMode(false);
    setSelectedConversationIDs([]);
  }, []);

  const toggleSelectionMode = React.useCallback(
    (checked: boolean | "indeterminate") => {
      const visibleConversationIDs = filteredItems.map((item) => item.publicID);

      if (checked) {
        setSelectionMode(true);
        setSelectedConversationIDs((current) => {
          const next = new Set(current);
          for (const id of visibleConversationIDs) {
            next.add(id);
          }
          return Array.from(next);
        });
        return;
      }

      exitSelectionMode();
    },
    [exitSelectionMode, filteredItems],
  );

  const selectedItems = React.useMemo(
    () => items.filter((item) => selectedConversationIDs.includes(item.publicID)),
    [items, selectedConversationIDs],
  );

  const selectedProjectID = React.useMemo<string | null>(() => {
    if (selectedItems.length === 0) {
      return null;
    }
    const firstProjectID = selectedItems[0]?.projectID ?? "";
    return selectedItems.every((item) => (item.projectID ?? "") === firstProjectID) ? firstProjectID : null;
  }, [selectedItems]);

  const moveSelectedToProject = React.useCallback(async (projectID?: string) => {
    if (selectedConversationIDs.length === 0 || movingSelectedToProject) {
      return;
    }

    const targetIDs = [...selectedConversationIDs];
    const targetIDSet = new Set(targetIDs);
    const targetProject = projects.find((project) => project.publicID === projectID);
    const patch: Partial<ConversationDTO> = {
      projectID: targetProject?.publicID ?? "",
      projectName: targetProject?.name ?? "",
    };
    setMovingSelectedToProject(true);
    try {
      const updated = await batchSetProjectByPublicIDs(targetIDs, projectID);
      if (updated !== targetIDs.length) {
        throw new Error("not all selected conversations were moved");
      }
      setItems((current) => current
        .map((item) => (targetIDSet.has(item.publicID) ? { ...item, ...patch } : item))
        .filter((item) => conversationMatchesRecentFilters(item, statusFilter, starredFilter, projectFilter)));
      setSelectedConversationIDs([]);
      setSelectionMode(false);
      toast.success(t("moveSelectedSuccess", {
        count: updated,
        project: targetProject?.name ?? t("projects.unassigned"),
      }));
    } catch (error) {
      toast.error(t("moveSelectedFailed"), {
        description: resolveErrorMessage(error, t("moveSelectedFailed")),
      });
    } finally {
      setMovingSelectedToProject(false);
    }
  }, [
    batchSetProjectByPublicIDs,
    movingSelectedToProject,
    projectFilter,
    projects,
    resolveErrorMessage,
    selectedConversationIDs,
    starredFilter,
    statusFilter,
    t,
  ]);

  const allSelectedArchived = React.useMemo(
    () => selectedItems.length > 0 && selectedItems.every((item) => isArchivedConversation(item)),
    [selectedItems],
  );

  const archiveSelected = React.useCallback(async () => {
    if (selectedItems.length === 0) {
      return;
    }

    const nextArchived = !allSelectedArchived;
    const targets = selectedItems.filter((item) => isArchivedConversation(item) !== nextArchived);
    const updates = (await runBulkActionInChunks({
      chunkSize: 10,
      items: targets,
      title: t("dialogs.bulk.pending"),
      runChunk: async (chunk) => {
        const updatedItems: Array<ConversationDTO | null> = [];
        for (const item of chunk) {
          updatedItems.push(await archiveByPublicID(item.publicID, nextArchived));
        }
        return updatedItems;
      },
    })).flat();

    setItems((current) => {
      let next = current;
      for (let index = 0; index < targets.length; index += 1) {
        const target = targets[index];
        const updated = updates[index];
        if (!updated) {
          continue;
        }
        if ((statusFilter === "active" && nextArchived) || (statusFilter === "archived" && !nextArchived)) {
          next = removeByPublicID(next, target.publicID);
          continue;
        }
        next = conversationMatchesRecentFilters(updated, statusFilter, starredFilter, projectFilter)
          ? upsertByPublicID(next, updated)
          : removeByPublicID(next, target.publicID);
      }
      return next;
    });
    setSelectedConversationIDs([]);
    setSelectionMode(false);
  }, [allSelectedArchived, archiveByPublicID, projectFilter, selectedItems, starredFilter, statusFilter, t]);

  const requestDeleteSelected = React.useCallback(() => {
    if (selectedConversationIDs.length === 0) {
      return;
    }

    setDeleteFiles(deleteFilesByDefault);
    setDeleteTarget({
      ids: [...selectedConversationIDs],
      label: t("selectedConversationCountLabel", { count: selectedConversationIDs.length }),
    });
  }, [deleteFilesByDefault, selectedConversationIDs, t]);

  const rowStates = React.useMemo<RecentRowState[]>(
    () =>
      filteredItems.map((item) => {
        const hovered = hoveredConversationID === item.publicID;
        const selected = selectedConversationIDs.includes(item.publicID);
        return {
          publicID: item.publicID,
          hovered,
          selected,
          highlighted: hovered || selected,
        };
      }),
    [filteredItems, hoveredConversationID, selectedConversationIDs],
  );

  const visibleSelectedCount = React.useMemo(
    () => filteredItems.filter((item) => selectedConversationIDs.includes(item.publicID)).length,
    [filteredItems, selectedConversationIDs],
  );

  const pageSelectionState = React.useMemo<boolean | "indeterminate">(() => {
    if (filteredItems.length === 0 || visibleSelectedCount === 0) {
      return false;
    }

    if (visibleSelectedCount === filteredItems.length) {
      return true;
    }

    return "indeterminate";
  }, [filteredItems.length, visibleSelectedCount]);

  return {
    items,
    filteredItems,
    normalizedQuery,
    loadingInitial,
    loadingMore,
    hasMore,
    loadMoreFailed,
    statusFilter,
    starredFilter,
    projectFilter,
    projects,
    query,
    isSelectionMode,
    selectedConversationIDs,
    selectedProjectID,
    movingSelectedToProject,
    hoveredConversationID,
    renameTarget,
    renameValue,
    renamingAutomatically,
    labelsTarget,
    deleteTarget,
    deleteFiles,
    rowStates,
    allSelectedArchived,
    pageSelectionState,
    loadMoreRef,
    onCreateConversation,
    setQuery,
    setStatusFilter,
    setStarredFilter,
    setProjectFilter: onProjectFilterChange,
    setHoveredConversationID,
    onToggleSelected,
    onToggleStar,
    onRename,
    onManageLabels,
    onUpdateLabels,
    onArchive,
    onSetProject,
    onDelete,
    setRenameValue,
    onRenameCommit,
    onAutoRename,
    closeRenameDialog: () => {
      setRenameTarget(null);
      setRenameValue("");
    },
    closeLabelsDialog: () => setLabelsTarget(null),
    confirmDelete,
    closeDeleteDialog,
    setDeleteFiles,
    toggleSelectionMode,
    archiveSelected,
    moveSelectedToProject,
    requestDeleteSelected,
    exitSelectionMode,
    enterSelectionMode: () => setSelectionMode(true),
    retryLoadMore,
  };
}
