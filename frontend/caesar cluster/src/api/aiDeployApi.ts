import axios from 'axios';

const aiClient = axios.create({
  baseURL: import.meta.env.VITE_AI_API_URL ?? 'http://localhost:8090',
  headers: { 'Content-Type': 'application/json' },
});

// ─── Shared types ──────────────────────────────────────────────────────────────

export interface DeploySubmission {
  service_name: string;
  docker_url: string;
  env_vars: Record<string, string>;
  role?: string;
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

export type ReviewStatus =
  | 'queued'
  | 'reviewing'
  | 'needs_confirmation'
  | 'escalated'
  | 'failed';

export interface ReviewSubmission {
  request_id: string;
  service_name: string;
  image: string;
  cpu_milli: number;
  ram_mb: number;
  compose_yaml?: string;
}

export interface ReviewResult {
  request_id: string;
  status: ReviewStatus;
  attempt_count: number;
  target_agent?: string;
  proposed_fix?: string;
  justification?: string;
  feedback_insight?: string;
  error_message?: string;
  updated_at: string;
}

export interface TelemetrySnapshot {
  latencies_ms: Record<string, number>;
  prompt_tokens: number;
  completion_tokens: number;
  retry_count: number;
  max_retries: number;
  circuit_status: string;
}

// ─── MOCK MODE (VITE_MOCK_AI=true) ────────────────────────────────────────────
// ตั้ง VITE_MOCK_AI=true ใน .env.local เพื่อทดสอบ UI flow ทั้งหมดโดยไม่ต้องรัน Go server เลย
// pipeline จะ simulate ทีละ stage พร้อม delay สมจริง

const isMock = import.meta.env.VITE_MOCK_AI === 'true';

function sleep(ms: number) {
  return new Promise<void>((r) => setTimeout(r, ms));
}

// ติดตามว่า request_id นี้ถูก poll ไปกี่ครั้งแล้ว เพื่อ simulate การ progress ของ pipeline
const mockPollCount = new Map<string, number>();

const mockDeployApi = {
  submit: async (payload: DeploySubmission): Promise<DeployResult> => {
    await sleep(300);
    const now = new Date().toISOString();
    return {
      job_id: `mock-dpl-${Date.now().toString(36)}`,
      service_name: payload.service_name,
      docker_url: payload.docker_url,
      env_vars: payload.env_vars,
      status: 'done',
      message: '[MOCK] Sandbox test passed — image pulled, 0 runtime errors detected',
      created_at: now,
      updated_at: now,
    };
  },
  getStatus: async (jobId: string): Promise<DeployResult> => {
    await sleep(200);
    const now = new Date().toISOString();
    return {
      job_id: jobId,
      service_name: 'mock-service',
      docker_url: 'mock:latest',
      env_vars: {},
      status: 'done',
      message: '[MOCK] Sandbox passed',
      created_at: now,
      updated_at: now,
    };
  },
};

const mockReviewApi = {
  submit: async (payload: ReviewSubmission): Promise<ReviewResult> => {
    await sleep(400);
    mockPollCount.set(payload.request_id, 0);
    return {
      request_id: payload.request_id,
      status: 'queued',
      attempt_count: 0,
      updated_at: new Date().toISOString(),
    };
  },

  // Simulate pipeline stage-by-stage: queued → sandbox → router → expert → evaluator → approved
  getStatus: async (requestId: string): Promise<ReviewResult> => {
    await sleep(700);
    const count = (mockPollCount.get(requestId) ?? 0) + 1;
    mockPollCount.set(requestId, count);
    const now = new Date().toISOString();

    // Stage 1: Sandbox running (count 1)
    if (count === 1) {
      return { request_id: requestId, status: 'reviewing', attempt_count: 0, updated_at: now };
    }
    // Stage 2: Router running (count 2) — sandbox found error, router analyzing
    if (count === 2) {
      return { request_id: requestId, status: 'reviewing', attempt_count: 0, updated_at: now };
    }
    // Stage 3: Expert running — attempt 1 (count 3)
    if (count === 3) {
      return {
        request_id: requestId,
        status: 'reviewing',
        attempt_count: 1,
        target_agent: 'backend_expert',
        updated_at: now,
      };
    }
    // Stage 4: Evaluator reviewing (count 4)
    if (count === 4) {
      return {
        request_id: requestId,
        status: 'reviewing',
        attempt_count: 1,
        target_agent: 'backend_expert',
        updated_at: now,
      };
    }
    // Stage 5: Pipeline approved → needs_confirmation (count 5+)
    mockPollCount.delete(requestId);
    return {
      request_id: requestId,
      status: 'needs_confirmation',
      attempt_count: 1,
      target_agent: 'backend_expert',
      proposed_fix:
        'เพิ่ม nil-check และ graceful error handling ก่อนใช้ database connection:\n\n' +
        'if conn == nil {\n' +
        '    log.Error("database connection is nil — restarting")\n' +
        '    return fmt.Errorf("db connection nil: ensure DB_URL env var is set")\n' +
        '}',
      justification:
        'Container ล่มเพราะ nil pointer dereference ใน database connection initialization ' +
        'ต้องเพิ่ม nil check ก่อน use และตรวจสอบ DB_URL env var ว่าถูก inject เข้ามาหรือไม่',
      feedback_insight:
        'ผ่านการตรวจสอบ Red-Team: nil-check ปลอดภัย ไม่มีช่องโหว่ ไม่ละเมิด Constitutional AI',
      updated_at: now,
    };
  },
};

const mockTelemetryApi = {
  get: async (): Promise<TelemetrySnapshot> => {
    await sleep(100);
    return {
      latencies_ms: { router: 241, expert: 612, evaluator: 338 },
      prompt_tokens: 1248,
      completion_tokens: 384,
      retry_count: 1,
      max_retries: 3,
      circuit_status: 'approved',
    };
  },
};

// ─── Real API implementations ──────────────────────────────────────────────────

const realDeployApi = {
  submit: async (payload: DeploySubmission): Promise<DeployResult> => {
    const res = await aiClient.post<DeployResult>('/api/deploy', payload);
    return res.data;
  },
  getStatus: async (jobId: string): Promise<DeployResult> => {
    const res = await aiClient.get<DeployResult>(`/api/deploy/${jobId}`);
    return res.data;
  },
};

const realReviewApi = {
  submit: async (payload: ReviewSubmission): Promise<ReviewResult> => {
    const res = await aiClient.post<ReviewResult>('/api/review', payload);
    return res.data;
  },
  getStatus: async (requestId: string): Promise<ReviewResult> => {
    const res = await aiClient.get<ReviewResult>(`/api/review/${requestId}`);
    return res.data;
  },
};

const realTelemetryApi = {
  get: async (): Promise<TelemetrySnapshot> => {
    const res = await aiClient.get<TelemetrySnapshot>('/api/telemetry');
    return res.data;
  },
};

// ─── Exported APIs — switch between real and mock via VITE_MOCK_AI ─────────────

export const aiDeployApi = isMock ? mockDeployApi : realDeployApi;
export const aiReviewApi = isMock ? mockReviewApi : realReviewApi;
export const aiTelemetryApi = isMock ? mockTelemetryApi : realTelemetryApi;
