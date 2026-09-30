export type UserUpstreamDTO = {
  id: number;
  name: string;
  baseURL: string;
  compatible: string;
  apiKeysMasked: string;
  status: string;
  modelsCount: number;
  activeModelsCount: number;
};

export type UserUpstreamModelDTO = {
  platformModelName: string;
  upstreamModelName: string;
  upstreamID: number;
  routeID: number;
  protocol: string;
};

export type UserRemoteModelDTO = {
  upstreamModelName: string;
  alreadyBound: boolean;
  alreadySynced: boolean;
};
