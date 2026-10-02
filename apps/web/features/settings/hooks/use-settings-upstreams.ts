"use client";

import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";

import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import { MODEL_CATALOG_CHANGED_EVENT } from "@/shared/api/model";
import { configureNativeSearchBatch, type NativeSearchResult } from "@/shared/api/native-search";
import {
  createUserUpstream,
  deleteUserUpstream,
  deleteUserUpstreamModel,
  importUserModels,
  listUserRemoteModels,
  listUserUpstreamModels,
  listUserUpstreams,
  updateUserUpstream,
} from "@/shared/api/upstreams";
import type { UserRemoteModelDTO, UserUpstreamDTO, UserUpstreamModelDTO } from "@/shared/api/upstreams-types";
import { useAuthSession } from "@/shared/auth/auth-session-context";

export const USER_UPSTREAM_SORT_OPTIONS = [
  { labelKey: "sort.idDesc", value: "id_desc" },
  { labelKey: "sort.idAsc", value: "id_asc" },
  { labelKey: "sort.nameAsc", value: "name_asc" },
  { labelKey: "sort.updatedDesc", value: "updated_desc" },
] as const;

export type UserUpstreamSortValue = (typeof USER_UPSTREAM_SORT_OPTIONS)[number]["value"];

export type UserUpstreamFormInput = {
  name: string;
  baseURL: string;
  compatible: string;
  apiKeys: string;
};

const DEFAULT_PAGE_SIZE = 20;
const MODEL_IMPORT_BATCH_SIZE = 100;

export function useSettingsUpstreams() {
  const { accessToken } = useAuthSession();
  const t = useTranslations("settings.upstreamsPage");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const [upstreams, setUpstreams] = React.useState<UserUpstreamDTO[]>([]);
  const [total, setTotal] = React.useState(0);
  const [addedModels, setAddedModels] = React.useState<Record<number, UserUpstreamModelDTO[]>>({});
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [detectingSearch, setDetectingSearch] = React.useState(false);
  const [searchResults, setSearchResults] = React.useState<Record<string, NativeSearchResult>>({});
  const [loadError, setLoadError] = React.useState("");
  const [query, setQuery] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState("");
  const [compatibleFilter, setCompatibleFilter] = React.useState("");
  const [sortValue, setSortValue] = React.useState<UserUpstreamSortValue>("id_desc");
  const [page, setPageState] = React.useState(1);
  const [pageSize, setPageSizeState] = React.useState(DEFAULT_PAGE_SIZE);
  const requestSeqRef = React.useRef(0);
  const syncInFlightRef = React.useRef(false);

  const reload = React.useCallback(async () => {
    const requestSeq = requestSeqRef.current + 1;
    requestSeqRef.current = requestSeq;
    setLoading(true);
    setLoadError("");
    try {
      const result = await listUserUpstreams(accessToken, {
        page,
        pageSize,
        query,
        status: statusFilter,
        compatible: compatibleFilter,
        sort: sortValue,
      });
      if (requestSeq !== requestSeqRef.current) {
        return false;
      }
      const models = await Promise.all(
        result.results.map(async (upstream): Promise<[number, UserUpstreamModelDTO[]]> => {
          const response = await listUserUpstreamModels(accessToken, upstream.id);
          return [upstream.id, response.items];
        }),
      );
      if (requestSeq !== requestSeqRef.current) {
        return false;
      }
      setUpstreams(result.results);
      setTotal(result.total);
      setAddedModels(Object.fromEntries(models));
      return true;
    } catch (error) {
      if (requestSeq !== requestSeqRef.current) {
        return false;
      }
      setLoadError(resolveErrorMessage(error));
      return false;
    } finally {
      if (requestSeq === requestSeqRef.current) {
        setLoading(false);
      }
    }
  }, [accessToken, compatibleFilter, page, pageSize, query, resolveErrorMessage, sortValue, statusFilter]);

  React.useEffect(() => {
    void reload();
  }, [reload]);

  React.useEffect(() => {
    setPageState(1);
  }, [compatibleFilter, query, sortValue, statusFilter]);

  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(page, pageCount);

  const setPage = React.useCallback((value: number) => {
    setPageState(value);
  }, []);

  const setPageSize = React.useCallback((value: number) => {
    setPageSizeState(value);
    setPageState(1);
  }, []);

  const mutate = React.useCallback(async (action: () => Promise<unknown>, successMessage: string) => {
    setSaving(true);
    try {
      await action();
      window.dispatchEvent(new Event(MODEL_CATALOG_CHANGED_EVENT));
      toast.success(successMessage);
      await reload();
      return true;
    } catch (error) {
      toast.error(t("saveFailed"), { description: resolveErrorMessage(error) });
      return false;
    } finally {
      setSaving(false);
    }
  }, [reload, resolveErrorMessage, t]);

  const addUpstream = React.useCallback(
    (input: UserUpstreamFormInput) => mutate(() => createUserUpstream(accessToken, input), t("upstreamAdded")),
    [accessToken, mutate, t],
  );

  const editUpstream = React.useCallback(
    (id: number, input: Partial<UserUpstreamFormInput>) =>
      mutate(() => updateUserUpstream(accessToken, id, input), t("upstreamUpdated")),
    [accessToken, mutate, t],
  );

  const remove = React.useCallback(
    (id: number) => mutate(() => deleteUserUpstream(accessToken, id), t("upstreamDeleted")),
    [accessToken, mutate, t],
  );

  const discover = React.useCallback(async (id: number): Promise<UserRemoteModelDTO[]> => {
    const result = await listUserRemoteModels(accessToken, id);
    return result.items;
  }, [accessToken]);

  const syncModels = React.useCallback(async (upstreamID: number, modelNames: string[]) => {
    if (syncInFlightRef.current) {
      return false;
    }
    syncInFlightRef.current = true;
    setSaving(true);
    let success = false;
    try {
      // 每次同步重新读取绑定，重试时只提交尚未完成的变更。
      const { items } = await listUserUpstreamModels(accessToken, upstreamID);
      const selectedNames = new Set(modelNames);
      const boundNames = new Set(items.map((model) => model.upstreamModelName));
      const additions = [...selectedNames].filter((name) => !boundNames.has(name));
      const removedRouteIDs = new Set(items
        .filter((model) => !selectedNames.has(model.upstreamModelName) && model.routeID > 0)
        .map((model) => model.routeID));
      const imported: UserUpstreamModelDTO[] = [];
      for (let offset = 0; offset < additions.length; offset += MODEL_IMPORT_BATCH_SIZE) {
        const result = await importUserModels(accessToken, upstreamID, additions.slice(offset, offset + MODEL_IMPORT_BATCH_SIZE));
        imported.push(...result.items);
      }
      // 图片等模型可以对应多条协议路由，取消模型时需要全部解除。
      for (const routeID of removedRouteIDs) {
        await deleteUserUpstreamModel(accessToken, upstreamID, routeID);
      }
      setDetectingSearch(true);
      await configureNativeSearchBatch(accessToken, upstreamID, imported, (model, result) => {
        setSearchResults((previous) => ({ ...previous, [`${upstreamID}:${model.upstreamModelName}`]: result }));
      });
      success = true;
    } catch (error) {
      toast.error(t("syncFailed"), { description: resolveErrorMessage(error) });
    } finally {
      // 部分操作成功后也刷新实际状态，保留弹窗中的选择供用户重试。
      const refreshed = await reload();
      if (success && !refreshed) {
        toast.error(t("loadFailed"));
        success = false;
      }
      window.dispatchEvent(new Event(MODEL_CATALOG_CHANGED_EVENT));
      setSaving(false);
      setDetectingSearch(false);
      syncInFlightRef.current = false;
    }
    if (success) {
      toast.success(t("modelsSynced"));
    }
    return success;
  }, [accessToken, reload, resolveErrorMessage, t]);

  const detectSearch = React.useCallback(async (upstreamID: number, modelNames: string[]) => {
    if (syncInFlightRef.current) return;
    syncInFlightRef.current = true;
    setSaving(true);
    setDetectingSearch(true);
    try {
      const { items } = await listUserUpstreamModels(accessToken, upstreamID);
      const selected = new Set(modelNames);
      await configureNativeSearchBatch(accessToken, upstreamID, items.filter((model) => selected.has(model.upstreamModelName)), (model, result) => {
        setSearchResults((previous) => ({ ...previous, [`${upstreamID}:${model.upstreamModelName}`]: result }));
      });
    } catch (error) {
      toast.error(t("searchDetectionFailed"), { description: resolveErrorMessage(error) });
    } finally {
      await reload();
      window.dispatchEvent(new Event(MODEL_CATALOG_CHANGED_EVENT));
      setSaving(false);
      setDetectingSearch(false);
      syncInFlightRef.current = false;
    }
  }, [accessToken, reload, resolveErrorMessage, t]);

  return {
    upstreams,
    total,
    addedModels,
    loading,
    saving,
    detectingSearch,
    searchResults,
    detectSearch,
    loadError,
    query,
    setQuery,
    statusFilter,
    setStatusFilter,
    compatibleFilter,
    setCompatibleFilter,
    sortValue,
    setSortValue,
    page: safePage,
    pageCount,
    pageSize,
    setPage,
    setPageSize,
    reload,
    addUpstream,
    editUpstream,
    discover,
    syncModels,
    remove,
  };
}
