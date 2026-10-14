import type { EmbeddingIndexStatusResponse, EmbeddingReindexResponse } from "@deeix/api-contract";
import { authedRequest } from "@/shared/api/authed-client";
import { pathParam } from "@/shared/api/http-client";
import type {
  AdminDoclingRuntimeView,
  AdminEmbeddingRuntimeView,
  AdminMinerURuntimeView,
  AdminRapidOCRRuntimeView,
  AdminTesseractRuntimeView,
  AdminTikaRuntimeView,
} from "@/features/admin/api/admin-types";
import type { PatchSettingsRequest, SettingItem, SettingsGrouped } from "@/shared/api/settings-types";

export async function listAdminSettings(accessToken: string): Promise<SettingsGrouped> {
  return authedRequest<SettingsGrouped>(
    "/api/v1/admin/settings",
    { accessToken },
    true,
  );
}

export async function listAdminSettingsByNamespace(
  accessToken: string,
  namespace: string,
): Promise<SettingItem[]> {
  return authedRequest<SettingItem[]>(
    `/api/v1/admin/settings/${pathParam(namespace)}`,
    { accessToken },
    true,
  );
}

export async function patchAdminSettings(
  accessToken: string,
  payload: PatchSettingsRequest,
): Promise<SettingsGrouped> {
  return authedRequest<SettingsGrouped>(
    "/api/v1/admin/settings",
    {
      method: "PATCH",
      accessToken,
      body: payload,
    },
    true,
  );
}

export async function getAdminTikaRuntime(accessToken: string): Promise<AdminTikaRuntimeView> {
  return authedRequest<AdminTikaRuntimeView>(
    "/api/v1/admin/settings/tika/runtime",
    { accessToken },
    true,
  );
}

export async function getAdminDoclingRuntime(accessToken: string): Promise<AdminDoclingRuntimeView> {
  return authedRequest<AdminDoclingRuntimeView>(
    "/api/v1/admin/settings/docling/runtime",
    { accessToken },
    true,
  );
}

export async function getAdminTesseractRuntime(accessToken: string): Promise<AdminTesseractRuntimeView> {
  return authedRequest<AdminTesseractRuntimeView>(
    "/api/v1/admin/settings/tesseract/runtime",
    { accessToken },
    true,
  );
}

export async function getAdminRapidOCRRuntime(accessToken: string): Promise<AdminRapidOCRRuntimeView> {
  return authedRequest<AdminRapidOCRRuntimeView>(
    "/api/v1/admin/settings/rapidocr/runtime",
    { accessToken },
    true,
  );
}

export async function getAdminMinerURuntime(accessToken: string): Promise<AdminMinerURuntimeView> {
  return authedRequest<AdminMinerURuntimeView>(
    "/api/v1/admin/settings/mineru/runtime",
    { accessToken },
    true,
  );
}

export async function getAdminEmbeddingRuntime(accessToken: string): Promise<AdminEmbeddingRuntimeView> {
  return authedRequest<AdminEmbeddingRuntimeView>(
    "/api/v1/admin/settings/embedding/runtime",
    { accessToken },
    true,
  );
}

export type AdminEmbeddingIndexStatus = EmbeddingIndexStatusResponse;

export async function getAdminEmbeddingStatus(
  accessToken: string,
  signal?: AbortSignal,
): Promise<AdminEmbeddingIndexStatus> {
  return authedRequest<AdminEmbeddingIndexStatus>(
    "/api/v1/admin/settings/embedding/status",
    { accessToken, signal },
    true,
  );
}

export async function triggerAdminEmbeddingReindex(
  accessToken: string,
  options: { includeEmpty?: boolean } = {},
): Promise<EmbeddingReindexResponse> {
  const path = options.includeEmpty
    ? "/api/v1/admin/settings/embedding/reindex?include_empty=true"
    : "/api/v1/admin/settings/embedding/reindex";
  return authedRequest<EmbeddingReindexResponse>(path, { method: "POST", accessToken }, true);
}
