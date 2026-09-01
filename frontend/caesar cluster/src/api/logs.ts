// ดึง log ของ service แบบสตรีม — เหมือนหน้า Logs ของ Google Cloud Run
//
// ใช้ fetch ตรงๆ ไม่ผ่าน axiosClient เพราะต้องอ่าน response ทีละก้อนด้วย ReadableStream
// ระหว่างที่ backend ยังส่งมาเรื่อยๆ (โหมด follow) ซึ่ง axios ในเบราว์เซอร์ไม่รองรับ
// (responseType: 'stream' ใช้ได้เฉพาะบน Node.js เท่านั้น)

import { useAuthStore } from '@/store/authStore';
import { API_URL } from '@/config/env';

export interface LogLine {
  // key เดิม (ก่อนตัด timestamp) ใช้เป็น React key กันบรรทัดที่เนื้อหาซ้ำกันเป๊ะ (เช่น mock
  // ที่วน "[mock] GET / 200 12ms" ซ้ำได้) แสดงผลชนกันจน React confuse ว่าเป็น element เดิม
  id: number;
  timestamp: string | null;
  text: string;
}

export interface StreamLogsOptions {
  tailLines?: number;
  sinceSeconds?: number;
  follow?: boolean;
  onLine: (line: LogLine) => void;
  signal: AbortSignal;
}

// backend ส่ง log แต่ละบรรทัดเป็น "<RFC3339Nano timestamp> <เนื้อ log>\n" เสมอ
// (ทั้ง mock และของจริงบนคลัสเตอร์ ดู backend/internal/controller/service_controller.go)
// แยกด้วยช่องว่างตัวแรกเท่านั้น เผื่อเนื้อ log เองมีช่องว่างอยู่ข้างในเยอะแค่ไหนก็ตาม
function parseLine(raw: string, id: number): LogLine {
  const sp = raw.indexOf(' ');
  if (sp === -1) return { id, timestamp: null, text: raw };
  const ts = raw.slice(0, sp);
  // ตรวจคร่าวๆ ว่าท่อนแรกหน้าตาเป็น timestamp จริง (ขึ้นต้นด้วยปี) ไม่ใช่ก็ถือว่าทั้งบรรทัดเป็นเนื้อ log
  // กันกรณี log ของแอปเองบังเอิญมีช่องว่างในตำแหน่งที่ทำให้ parse ผิดรูป
  if (!/^\d{4}-\d{2}-\d{2}T/.test(ts)) return { id, timestamp: null, text: raw };
  return { id, timestamp: ts, text: raw.slice(sp + 1) };
}

/**
 * เปิดสตรีม log ของ service หนึ่งตัว เรียก onLine ทุกครั้งที่ได้บรรทัดใหม่
 * คืน Promise ที่ resolve เมื่อสตรีมปิดปกติ (จบเอง หรือ signal ถูก abort)
 * โยน error ออกมาถ้าเปิดสตรีมไม่สำเร็จตั้งแต่ต้น (เช่น 404/403) ให้ผู้เรียกจัดการเอง
 */
export async function streamServiceLogs(serviceId: number, opts: StreamLogsOptions): Promise<void> {
  const params = new URLSearchParams();
  if (opts.tailLines) params.set('tail', String(opts.tailLines));
  if (opts.sinceSeconds) params.set('since', String(opts.sinceSeconds));
  if (opts.follow) params.set('follow', 'true');

  const token = useAuthStore.getState().token;
  const res = await fetch(`${API_URL}/services/${serviceId}/logs?${params}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    signal: opts.signal,
  });

  if (!res.ok) {
    // error response ของ endpoint นี้เป็น JSON ธรรมดา (ยังไม่ทันเริ่ม stream)
    let message = `ดึง log ไม่สำเร็จ (HTTP ${res.status})`;
    try {
      const body = (await res.json()) as { error?: { message?: string } };
      if (body.error?.message) message = body.error.message;
    } catch {
      // response ไม่ใช่ JSON ก็ใช้ข้อความ default ด้านบนต่อไป
    }
    throw new Error(message);
  }
  if (!res.body) throw new Error('เบราว์เซอร์นี้ไม่รองรับการอ่าน log แบบสตรีม');

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let nextId = 0;

  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? ''; // ท่อนสุดท้ายอาจยังมาไม่ครบบรรทัด เก็บไว้รอต่อรอบหน้า

      for (const line of lines) {
        if (line === '') continue;
        opts.onLine(parseLine(line, nextId++));
      }
    }
    if (buffer !== '') opts.onLine(parseLine(buffer, nextId++)); // บรรทัดสุดท้ายที่ไม่มี \n ปิดท้าย
  } catch (err) {
    // ปิดหน้าเว็บ/เปลี่ยนหน้าทำให้ signal ถูก abort — reader.read() โยน AbortError ออกมา
    // ถือเป็นการจบสตรีมตามปกติ ไม่ใช่ความผิดพลาดที่ต้องเด้ง error ให้ผู้ใช้เห็น
    if (err instanceof DOMException && err.name === 'AbortError') return;
    throw err;
  } finally {
    reader.releaseLock();
  }
}
