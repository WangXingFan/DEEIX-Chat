"use client";

import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";

import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import { MODEL_CATALOG_CHANGED_EVENT } from "@/shared/api/model";
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

export function useSettingsUpstreams() {
  const { accessToken } = useAuthSession();
  const t = useTranslations("settings.upstreamsPage");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const [upstreams, setUpstreams] = React.useState<UserUpstreamDTO[]>([]);
  const [total, setTotal] = React.useState(0);
  const [addedModels, setAddedModels] = React.useState<Record<number, UserUpstreamModelDTO[]>>({});
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);
  const [loadError, setLoadError] = React.useState("");
  const [query, setQuery] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState("");
  const [compatibleFilter, setCompatibleFilter] = React.useState("");
  const [sortValue, setSortValue] = React.useState<UserUpstreamSortValue>("id_desc");
  const [page, setPageState] = React.useState(1);
  const [pageSize, setPageSizeState] = React.useState(DEFAULT_PAGE_SIZE);
  const requestSeqRef = React.useRef(0);

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
        return;
      }
      const models = await Promise.all(
        result.results.map(async (upstream): Promise<[number, UserUpstreamModelDTO[]]> => {
          const response = await listUserUpstreamModels(accessToken, upstream.id);
          return [upstream.id, response.items];
        }),
      );
      if (requestSeq !== requestSeqRef.current) {
        return;
      }
      setUpstreams(result.results);
      setTotal(result.total);
      setAddedModels(Object.fromEntries(models));
    } catch (error) {
      if (requestSeq !== requestSeqRef.current) {
        return;
      }
      setLoadError(resolveErrorMessage(error));
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

  const addModels = React.useCallback(
    (id: number, modelNames: string[]) => mutate(() => importUserModels(accessToken, id, modelNames), t("modelsAdded")),
    [accessToken, mutate, t],
  );

  const removeModel = React.useCallback(
    (upstreamID: number, routeID: number) =>
      mutate(() => deleteUserUpstreamModel(accessToken, upstreamID, routeID), t("modelRemoved")),
    [accessToken, mutate, t],
  );

  return {
    upstreams,
    total,
    addedModels,
    loading,
    saving,
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
    addModels,
    removeModel,
    remove,
  };
}
