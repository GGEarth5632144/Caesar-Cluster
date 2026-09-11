import { create } from 'zustand';

import type { SearchFieldDef } from '@/lib/search';

// ตัวกลางระหว่าง "ช่องค้นหาบน Topbar" กับ "หน้าที่กำลังเปิดอยู่"
//
// Topbar อยู่ใน DashboardLayout ซึ่งเป็นคนละต้นไม้กับ <Outlet/> ที่ render หน้าจริง
// จะส่ง prop ลงไปตรงๆ ไม่ได้ และการยก state ของทุกหน้าขึ้นไปไว้บน layout ก็ไม่เข้าท่า
// (layout ต้องรู้จักข้อมูลทุกหน้า) เลยใช้ store เป็นจุดนัดพบแทน:
//
//   หน้า  -> registerScope() บอกว่า "หน้านี้ค้นอะไรได้บ้าง" ตอน mount
//   Topbar -> อ่าน scope มา render placeholder/ตัวกรอง แล้วเขียน query กลับ
//   หน้า  -> อ่าน query ไปกรองข้อมูล แล้ว reportCount() ให้ Topbar โชว์ "เจอ n จาก m"
//
// หน้าไหนไม่ register (เช่น Dashboard, Settings) scope จะเป็น null
// Topbar จะสลับไปเป็นโหมดค้นหน้า/กระโดดไปหน้าอื่นแทนโดยอัตโนมัติ

/** ปุ่มกรองสำเร็จรูปที่โชว์ใต้ช่องค้นหา — กดแล้วเท่ากับพิมพ์ "fieldId:value" เอง */
export interface QuickFilterDef {
  /** ต้องตรงกับ id ของ SearchFieldDef ตัวใดตัวหนึ่งใน scope */
  fieldId: string;
  label: string;
  options: { value: string; label: string }[];
  /** true = เลือกได้หลายค่าพร้อมกัน (กลายเป็น fieldId:a|b) */
  multi?: boolean;
}

export interface SearchScope {
  /** ไอดีของหน้า ใช้แยกประวัติการค้นหาและใช้เช็คว่า scope เปลี่ยนหน้าแล้วหรือยัง */
  id: string;
  /** ชื่อสิ่งที่กำลังค้น เอาไปประกอบข้อความ เช่น "เจอ 3 จาก 12 บริการ" */
  noun: string;
  placeholder: string;
  fields: SearchFieldDef<any>[];
  quickFilters?: QuickFilterDef[];
  /** ตัวอย่างคิวรีที่กดแล้วเติมลงช่องได้เลย — ช่วยให้ผู้ใช้เห็นว่าหน้านี้ค้นอะไรได้ */
  examples?: string[];
}

const RECENT_KEY = 'caesar.search.recent';
const RECENT_LIMIT = 6;

type RecentMap = Record<string, string[]>;

function loadRecent(): RecentMap {
  try {
    const raw = localStorage.getItem(RECENT_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as unknown;
    // ข้อมูลใน localStorage แก้มือได้ ถ้ารูปร่างไม่ตรงก็เริ่มใหม่ดีกว่าปล่อยให้พังตอนใช้
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {};
    return parsed as RecentMap;
  } catch {
    return {};
  }
}

function saveRecent(map: RecentMap) {
  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify(map));
  } catch {
    // โหมดส่วนตัว/พื้นที่เต็ม — ประวัติการค้นหาหายไปไม่ใช่เรื่องคอขาดบาดตาย
  }
}

interface SearchState {
  scope: SearchScope | null;
  query: string;
  /** ฟิลด์ที่ผู้ใช้ปิดไม่ให้คำเปล่าไปค้น (เก็บแยกตาม scope id) */
  disabledFields: Record<string, string[]>;
  /** จำนวนที่ค้นเจอ / จำนวนทั้งหมด — หน้ารายงานกลับมาให้ Topbar โชว์ */
  matchCount: number | null;
  totalCount: number | null;
  recent: RecentMap;
  /** ตัวนับที่เพิ่มขึ้นทุกครั้งที่มีใครขอให้เคอร์เซอร์ไปอยู่ที่ช่องค้นหา
   *  ใช้ตัวนับแทน boolean เพราะต้องสั่งซ้ำได้เรื่อยๆ โดยไม่ต้องรีเซ็ตกลับเป็น false ก่อน */
  focusToken: number;

  requestFocus: () => void;
  setQuery: (query: string) => void;
  clearQuery: () => void;
  registerScope: (scope: SearchScope) => void;
  unregisterScope: (scopeId: string) => void;
  reportCount: (match: number, total: number) => void;
  toggleField: (fieldId: string) => void;
  resetFields: () => void;
  /** เพิ่ม/ถอนค่าของตัวกรองสำเร็จรูป โดยแก้ข้อความในช่องค้นหาให้ตรงกัน */
  toggleQuickFilter: (filter: QuickFilterDef, value: string) => void;
  commitRecent: () => void;
  removeRecent: (query: string) => void;
}

/** อ่านค่าของ "fieldId:a|b" ที่อยู่ในคิวรีตอนนี้ออกมาเป็น array */
export function readFilterValues(query: string, fieldId: string): string[] {
  const found = query
    .split(/\s+/)
    .find((token) => token.toLowerCase().startsWith(`${fieldId.toLowerCase()}:`));
  if (!found) return [];
  return found
    .slice(fieldId.length + 1)
    .split(/[|,]/)
    .map((v) => v.trim())
    .filter(Boolean);
}

/** เขียนค่าตัวกรองกลับลงคิวรี — ค่าว่างแปลว่าถอดตัวกรองนั้นออกทั้งอัน */
function writeFilterValues(query: string, fieldId: string, values: string[]): string {
  const kept = query
    .split(/\s+/)
    .filter((token) => token && !token.toLowerCase().startsWith(`${fieldId.toLowerCase()}:`));
  if (values.length > 0) kept.push(`${fieldId}:${values.join('|')}`);
  return kept.join(' ');
}

export const useSearchStore = create<SearchState>((set, get) => ({
  scope: null,
  query: '',
  disabledFields: {},
  matchCount: null,
  totalCount: null,
  recent: loadRecent(),
  focusToken: 0,

  requestFocus: () => set((state) => ({ focusToken: state.focusToken + 1 })),

  setQuery: (query) => set({ query }),

  clearQuery: () => set({ query: '' }),

  registerScope: (scope) =>
    set((state) => {
      // เข้าหน้าเดิมซ้ำ (เช่น re-render จาก data refresh) — ห้ามล้างคำที่ผู้ใช้พิมพ์ค้างไว้
      if (state.scope?.id === scope.id) return { scope };
      return { scope, query: '', matchCount: null, totalCount: null };
    }),

  unregisterScope: (scopeId) =>
    set((state) => {
      // หน้าใหม่ register ก่อนหน้าเก่า unmount ได้ (React ยิง effect ของลูกใหม่ก่อน)
      // ถ้าไม่เช็ค id ตรงนี้ หน้าเก่าจะลบ scope ของหน้าใหม่ทิ้ง แล้วช่องค้นหาจะตายไปเฉยๆ
      if (state.scope?.id !== scopeId) return state;
      return { scope: null, query: '', matchCount: null, totalCount: null };
    }),

  reportCount: (match, total) =>
    set((state) =>
      state.matchCount === match && state.totalCount === total
        ? state
        : { matchCount: match, totalCount: total },
    ),

  toggleField: (fieldId) =>
    set((state) => {
      const scopeId = state.scope?.id;
      if (!scopeId) return state;
      const current = state.disabledFields[scopeId] ?? [];
      const next = current.includes(fieldId)
        ? current.filter((id) => id !== fieldId)
        : [...current, fieldId];
      return { disabledFields: { ...state.disabledFields, [scopeId]: next } };
    }),

  resetFields: () =>
    set((state) => {
      const scopeId = state.scope?.id;
      if (!scopeId) return state;
      const { [scopeId]: _removed, ...rest } = state.disabledFields;
      return { disabledFields: rest };
    }),

  toggleQuickFilter: (filter, value) => {
    const { query } = get();
    const current = readFilterValues(query, filter.fieldId);
    const has = current.some((v) => v.toLowerCase() === value.toLowerCase());

    let next: string[];
    if (filter.multi) {
      next = has ? current.filter((v) => v.toLowerCase() !== value.toLowerCase()) : [...current, value];
    } else {
      // ตัวกรองค่าเดียว: กดตัวที่เลือกอยู่ซ้ำ = ยกเลิก, กดตัวอื่น = เปลี่ยนไปเป็นตัวนั้น
      next = has ? [] : [value];
    }

    set({ query: writeFilterValues(query, filter.fieldId, next).trim() });
  },

  commitRecent: () => {
    const { query, scope, recent } = get();
    const trimmed = query.trim();
    if (!trimmed || !scope) return;

    const key = scope.id;
    const previous = recent[key] ?? [];
    // ค้นซ้ำคำเดิมต้องเด้งขึ้นบนสุด ไม่ใช่มีสองบรรทัดเหมือนกัน
    const next = [trimmed, ...previous.filter((q) => q !== trimmed)].slice(0, RECENT_LIMIT);
    const map = { ...recent, [key]: next };
    saveRecent(map);
    set({ recent: map });
  },

  removeRecent: (query) => {
    const { scope, recent } = get();
    if (!scope) return;
    const key = scope.id;
    const next = (recent[key] ?? []).filter((q) => q !== query);
    const map = { ...recent, [key]: next };
    saveRecent(map);
    set({ recent: map });
  },
}));
