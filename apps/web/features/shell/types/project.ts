export type ProjectDraft = {
  publicID?: string;
  name: string;
  systemPrompt: string;
  defaultModel: string;
  defaultSkillIDs: number[];
  defaultKnowledgeBaseIDs: string[];
};
