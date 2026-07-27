import axios from 'axios';

const aiClient = axios.create({
  baseURL: import.meta.env.VITE_AI_API_URL ?? 'http://localhost:8090',
  headers: { 'Content-Type': 'application/json' },
});

export interface DeploySubmission {
  service_name: string;
  docker_url: string;
  env_vars: Record<string, string>;
}

export interface DeployResult {
  job_id: string;
  service_name: string;
  docker_url: string;
  env_vars: Record<string, string>;
  status: string;
  message: string;
  created_at: string;
  updated_at: string;
}

export const aiDeployApi = {
  submit: async (payload: DeploySubmission): Promise<DeployResult> => {
    const res = await aiClient.post<DeployResult>('/api/deploy', payload);
    return res.data;
  },

  getStatus: async (jobId: string): Promise<DeployResult> => {
    const res = await aiClient.get<DeployResult>(`/api/deploy/${jobId}`);
    return res.data;
  },
};
