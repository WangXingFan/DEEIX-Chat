import { authedRequest } from "@/shared/api/authed-client";
import type { UserRemoteModelDTO, UserUpstreamDTO, UserUpstreamModelDTO } from "@/shared/api/upstreams-types";

type ListResponse = { total: number; results: UserUpstreamDTO[] };
type RemoteResponse = { total: number; items: UserRemoteModelDTO[]; snapshotID: string };

export type ListUserUpstreamsOptions = {
  page?: number;
  pageSize?: number;
  query?: string;
  status?: string;
  compatible?: string;
  sort?: string;
};

export function listUserUpstreams(accessToken: string, options: ListUserUpstreamsOptions = {}) {
  const page = options.page && options.page > 0 ? options.page : 1;
  const pageSize = options.pageSize && options.pageSize > 0 ? options.pageSize : 20;
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
  const query = options.query?.trim();
  if (query) params.set("q", query);
  if (options.status) params.set("status", options.status);
  if (options.compatible) params.set("compatible", options.compatible);
  if (options.sort) params.set("sort", options.sort);
  return authedRequest<ListResponse>(`/api/v1/upstreams?${params.toString()}`, { accessToken }, true);
}

export function createUserUpstream(accessToken: string, input: { name: string; baseURL: string; compatible: string; apiKeys: string }) {
  return authedRequest<{ upstream: UserUpstreamDTO }>("/api/v1/upstreams", {
    accessToken,
    method: "POST",
    body: input,
  }, true);
}

export function updateUserUpstream(accessToken: string, id: number, input: Partial<{ name: string; baseURL: string; compatible: string; apiKeys: string; status: string }>) {
  return authedRequest<{ upstream: UserUpstreamDTO }>(`/api/v1/upstreams/${id}`, {
    accessToken,
    method: "PATCH",
    body: input,
  }, true);
}

export function deleteUserUpstream(accessToken: string, id: number) {
  return authedRequest<unknown>(`/api/v1/upstreams/${id}`, { accessToken, method: "DELETE" }, true);
}

export function listUserRemoteModels(accessToken: string, id: number) {
  return authedRequest<RemoteResponse>(`/api/v1/upstreams/${id}/models/remote`, { accessToken }, true);
}

export function importUserModels(accessToken: string, id: number, modelNames: string[]) {
  return authedRequest<{ items: UserUpstreamModelDTO[] }>(`/api/v1/upstreams/${id}/models`, {
    accessToken,
    method: "POST",
    body: { modelNames },
  }, true);
}

export function listUserUpstreamModels(accessToken: string, id: number) {
  return authedRequest<{ items: UserUpstreamModelDTO[] }>(`/api/v1/upstreams/${id}/models`, { accessToken }, true);
}

export function deleteUserUpstreamModel(accessToken: string, id: number, routeID: number) {
  return authedRequest<unknown>(`/api/v1/upstreams/${id}/models/${routeID}`, { accessToken, method: "DELETE" }, true);
}
