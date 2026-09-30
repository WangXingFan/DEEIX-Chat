"use client";

import * as React from "react";
import { toast } from "sonner";

import { useAuthSession } from "@/shared/auth/auth-session-context";
import { MODEL_CATALOG_CHANGED_EVENT } from "@/shared/api/model";
import { createUserUpstream, deleteUserUpstream, importUserModels, listUserRemoteModels, listUserUpstreams } from "@/shared/api/upstreams";
import type { UserRemoteModelDTO, UserUpstreamDTO } from "@/shared/api/upstreams-types";

export function useSettingsUpstreams() {
  const { accessToken } = useAuthSession();
  const [upstreams, setUpstreams] = React.useState<UserUpstreamDTO[]>([]);
  const [remoteModels, setRemoteModels] = React.useState<Record<number, UserRemoteModelDTO[]>>({});
  const [loading, setLoading] = React.useState(true);

  const reload = React.useCallback(async () => {
    setLoading(true);
    try {
      const result = await listUserUpstreams(accessToken);
      setUpstreams(result.results);
    } finally {
      setLoading(false);
    }
  }, [accessToken]);

  React.useEffect(() => {
    void reload();
  }, [reload]);

  const addUpstream = React.useCallback(async (input: { name: string; baseURL: string; compatible: string; apiKeys: string }) => {
    await createUserUpstream(accessToken, input);
    await reload();
    window.dispatchEvent(new Event(MODEL_CATALOG_CHANGED_EVENT));
    toast.success("Upstream added");
  }, [accessToken, reload]);

  const discover = React.useCallback(async (id: number) => {
    const result = await listUserRemoteModels(accessToken, id);
    setRemoteModels((current) => ({ ...current, [id]: result.items }));
  }, [accessToken]);

  const addModels = React.useCallback(async (id: number, modelNames: string[]) => {
    await importUserModels(accessToken, id, modelNames);
    await reload();
    await discover(id);
    window.dispatchEvent(new Event(MODEL_CATALOG_CHANGED_EVENT));
    toast.success("Models added");
  }, [accessToken, discover, reload]);

  const remove = React.useCallback(async (id: number) => {
    await deleteUserUpstream(accessToken, id);
    await reload();
    window.dispatchEvent(new Event(MODEL_CATALOG_CHANGED_EVENT));
    toast.success("Upstream deleted");
  }, [accessToken, reload]);

  return { upstreams, remoteModels, loading, reload, addUpstream, discover, addModels, remove };
}
