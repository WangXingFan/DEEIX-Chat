export type UserUpstreamDTO = {
  id: number;
  name: string;
  baseURL: string;
  compatible: string;
  apiKeysMasked: string;
  status: string;
  modelsCount: number;
  activeModelsCount: number;
  circuitOpen: boolean;
  createdAt: string;
  updatedAt: string;
};

export type UserUpstreamModelDTO = {
  platformModelName: string;
  upstreamModelName: string;
  upstreamID: number;
  routeID: number;
  protocol: string;
  routeStatus: string;
  upstreamModelStatus: string;
  circuitOpen: boolean;
};

export type UserRemoteModelDTO = {
  upstreamModelName: string;
  alreadyBound: boolean;
  alreadySynced: boolean;
};
