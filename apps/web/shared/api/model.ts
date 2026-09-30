import { authedRequest } from "@/shared/api/authed-client";
import type { PublicModelDTO } from "@/shared/api/model-types";

export const MODEL_CATALOG_CHANGED_EVENT = "deeix:model-catalog-changed";

export async function listPublicModels(accessToken: string, signal?: AbortSignal): Promise<PublicModelDTO[]> {
  return authedRequest<PublicModelDTO[]>(
    "/api/v1/models",
    {
      accessToken,
      signal,
    },
    true,
  );
}
