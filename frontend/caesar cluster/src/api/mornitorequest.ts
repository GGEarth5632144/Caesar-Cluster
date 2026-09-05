// mornitorequest.ts
import axiosClient from './axiosClient';

export interface NodeTelemetry {
    ID: number;
    NodeName: string;
    Temperature: number;
    RamUsedMB: number;
    IsUp: number;
    Procs: number;
    UpdatedAt: string; 
}

export interface ClusterHistoryData {
  time: string;
  avgTemp: number;
  totalRam: number;
  onlineNodes: number;
}

export const nodetelemetry = {
    getAll: async () => {
    // เปลี่ยนจาก ApiResponse<NodeTelemetry[]> เป็น NodeTelemetry[] ตรงๆ
    const response = await axiosClient.get<NodeTelemetry[]>('/telemetry');
    return response.data; 
  },
  getHistory: async (range: string): Promise<ClusterHistoryData[]> => {
    const response = await axiosClient.get(`/telemetry/history?range=${range}`);
    return response.data;
  },
};



