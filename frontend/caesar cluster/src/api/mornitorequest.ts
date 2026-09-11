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

export interface PowerNode {
  ModbusID: number;
  Status: string;
  Volt: number;
  Amp: number;
  Hz: number;
  PF: number;
  Whr: number;
  Watt: number;
  UpdatedAt: string;
}

export interface ClusterHistoryData {
  time: string;
  avgTemp: number;
  totalRam: number;
  onlineNodes: number;
}

export interface PowerHistoryData {
  time: string;
  totalWatt: number;
  totalAmp: number;
  avgVolt: number;
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
  getAllPower: async () => {
      const response = await axiosClient.get<PowerNode[]>('/power');
      return response.data;
  },
  getPowerHistory: async (timeRange: string = "1h") => {
    const response = await axiosClient.get<PowerHistoryData[]>(`/power/history?timeRange=${timeRange}`);
    return response.data;
  },
};



