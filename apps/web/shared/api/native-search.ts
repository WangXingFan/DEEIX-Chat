import { authedRequest } from "@/shared/api/authed-client";
import type { NativeSearchResponse } from "@deeix/api-contract";

export type NativeSearchResult = NativeSearchResponse;

export function configureNativeSearch(accessToken: string, upstreamID: number, routeID: number, admin = false, signal?: AbortSignal) {
  const prefix = admin ? "/api/v1/admin/llm" : "/api/v1";
  return authedRequest<NativeSearchResult>(`${prefix}/upstreams/${upstreamID}/models/${routeID}/native-search`, {
    accessToken,
    method: "POST",
    signal,
  });
}

// Each model gets a separate bounded request, so large imports do not hold one HTTP request open.
export async function configureNativeSearchBatch<T extends { routeID: number; upstreamModelName: string }>(
  accessToken: string,
  upstreamID: number,
  models: T[],
  onResult: (model: T, result: NativeSearchResult) => void,
  admin = false,
  signal?: AbortSignal,
) {
  const uniqueModels = [...new Map(models.filter((model) => model.routeID > 0).map((model) => [model.upstreamModelName, model])).values()];
  for (let offset = 0; offset < uniqueModels.length; offset += 3) {
    signal?.throwIfAborted();
    const batch = uniqueModels.slice(offset, offset + 3);
    const results = await Promise.allSettled(batch.map((model) => configureNativeSearch(accessToken, upstreamID, model.routeID, admin, signal)));
    signal?.throwIfAborted();
    results.forEach((result, index) => {
      onResult(batch[index], result.status === "fulfilled" ? result.value : { status: "unavailable", protocol: "", reason: "request_failed" });
    });
  }
}
