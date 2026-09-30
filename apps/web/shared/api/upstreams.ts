import { authedRequest } from "@/shared/api/authed-client";
import type { UserRemoteModelDTO, UserUpstreamDTO, UserUpstreamModelDTO } from "@/shared/api/upstreams-types";

type ListResponse = { total: number; results: UserUpstreamDTO[] };
type RemoteResponse = { total: number; items: UserRemoteModelDTO[]; snapshotID: string };

export function listUserUpstreams(accessToken: string) {
  return authedRequest<ListResponse>("/api/v1/upstreams?page=1&page_size=100", { accessToken }, true);
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
