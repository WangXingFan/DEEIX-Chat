import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";

import { useShellProjectCatalog } from "@/features/shell/hooks/use-shell-project-catalog";
import type { ProjectDraft } from "@/features/shell/types/project";
import { listVisibleKnowledgeBases } from "@/shared/api/knowledge-bases";
import type { KnowledgeBaseDTO } from "@/shared/api/knowledge-bases-types";
import { listPublicModels } from "@/shared/api/model";
import type { PublicModelDTO } from "@/shared/api/model-types";
import { listVisibleSkills } from "@/shared/api/skills";
import type { SkillSummaryDTO } from "@/shared/api/skills-types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

function getSkillID(skill: SkillSummaryDTO): number {
  return skill.id;
}

function getKnowledgeBaseID(knowledgeBase: KnowledgeBaseDTO): string {
  return knowledgeBase.publicID;
}

/**
 * Loads the catalogs behind the project dialog's default pickers (models, skills,
 * knowledge bases) while the dialog is open, and prunes draft selections that are
 * no longer available.
 */
export function useShellProjectDialogDefaults({
  open,
  draft,
  setDraft,
  knowledgeBaseEnabled,
}: {
  open: boolean;
  draft: ProjectDraft | null;
  setDraft: React.Dispatch<React.SetStateAction<ProjectDraft | null>>;
  knowledgeBaseEnabled: boolean;
}) {
  const t = useTranslations("recent.projects");
  const [modelCatalogLoading, setModelCatalogLoading] = React.useState(false);
  const [models, setModels] = React.useState<PublicModelDTO[]>([]);

  React.useEffect(() => {
    if (!open) {
      setModelCatalogLoading(false);
      setModels([]);
      return;
    }

    const controller = new AbortController();
    setModelCatalogLoading(true);
    void (async () => {
      try {
        const token = await resolveAccessToken();
        if (!token) {
          throw new Error("missing access token");
        }
        const items = await listPublicModels(token, controller.signal);
        if (!controller.signal.aborted) {
          setModels(items);
        }
      } catch {
        if (!controller.signal.aborted) {
          setModels([]);
          toast.error(t("defaultModelsLoadFailed"));
        }
      } finally {
        if (!controller.signal.aborted) {
          setModelCatalogLoading(false);
        }
      }
    })();
    return () => {
      controller.abort();
    };
  }, [open, t]);

  const handleCatalogLoadError = React.useCallback(() => {
    toast.error(t("defaultsLoadFailed"));
  }, [t]);
  const handleSkillIDsResolved = React.useCallback((
    requestedIDs: number[],
    availableIDs: ReadonlySet<number>,
  ) => {
    const requestedIDSet = new Set(requestedIDs);
    setDraft((current) => {
      if (!current) return current;
      const defaultSkillIDs = current.defaultSkillIDs.filter(
        (id) => !requestedIDSet.has(id) || availableIDs.has(id),
      );
      return defaultSkillIDs.length === current.defaultSkillIDs.length
        ? current
        : { ...current, defaultSkillIDs };
    });
  }, [setDraft]);
  const handleKnowledgeBaseIDsResolved = React.useCallback((
    requestedIDs: string[],
    availableIDs: ReadonlySet<string>,
  ) => {
    const requestedIDSet = new Set(requestedIDs);
    setDraft((current) => {
      if (!current) return current;
      const defaultKnowledgeBaseIDs = current.defaultKnowledgeBaseIDs
        .filter((id) => !requestedIDSet.has(id) || availableIDs.has(id))
        .slice(0, 8);
      return defaultKnowledgeBaseIDs.length === current.defaultKnowledgeBaseIDs.length &&
        defaultKnowledgeBaseIDs.every((id, index) => id === current.defaultKnowledgeBaseIDs[index])
        ? current
        : { ...current, defaultKnowledgeBaseIDs };
    });
  }, [setDraft]);
  const skillCatalog = useShellProjectCatalog({
    open,
    selectedIDs: draft?.defaultSkillIDs ?? [],
    loadPage: listVisibleSkills,
    getID: getSkillID,
    onSelectedIDsResolved: handleSkillIDsResolved,
    onError: handleCatalogLoadError,
  });
  const knowledgeBaseCatalog = useShellProjectCatalog({
    open: open && knowledgeBaseEnabled,
    selectedIDs: draft?.defaultKnowledgeBaseIDs.slice(0, 8) ?? [],
    loadPage: listVisibleKnowledgeBases,
    getID: getKnowledgeBaseID,
    onSelectedIDsResolved: handleKnowledgeBaseIDsResolved,
    onError: handleCatalogLoadError,
  });

  return {
    modelCatalogLoading,
    models,
    skillCatalog,
    knowledgeBaseCatalog,
  };
}
