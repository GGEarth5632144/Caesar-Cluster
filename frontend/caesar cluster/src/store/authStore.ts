import { create } from 'zustand';

import type { AuthUser } from '@/api/authApi';

const STORAGE_KEY = 'caesar-cluster-auth';

interface StoredAuth {
  token: string;
  user: AuthUser;
}

function readStoredAuth(): StoredAuth | null {
  const raw = localStorage.getItem(STORAGE_KEY) ?? sessionStorage.getItem(STORAGE_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as StoredAuth;
  } catch {
    return null;
  }
}

const initial = readStoredAuth();

interface AuthState {
  token: string | null;
  user: AuthUser | null;
  setAuth: (token: string, user: AuthUser, remember: boolean) => void;
  refreshUser: (user: AuthUser) => void;
  logout: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  token: initial?.token ?? null,
  user: initial?.user ?? null,
  setAuth: (token, user, remember) => {
    localStorage.removeItem(STORAGE_KEY);
    sessionStorage.removeItem(STORAGE_KEY);
    const storage = remember ? localStorage : sessionStorage;
    storage.setItem(STORAGE_KEY, JSON.stringify({ token, user }));
    set({ token, user });
  },
  // อัปเดต user ที่ cache ไว้โดยไม่ต้อง login ใหม่ — ใช้ตอนสถานะเปลี่ยนหลัง login แล้ว
  // เช่น accept คำเชิญเข้ากลุ่มทำให้ namespace_id เปลี่ยน แต่ token ยังใช้ได้เหมือนเดิม
  // เขียนกลับ storage ตัวเดิมที่ setAuth เคยเลือกไว้ (local ถ้าติ๊ก remember, ไม่งั้น session)
  refreshUser: (user) => {
    const storage = localStorage.getItem(STORAGE_KEY) ? localStorage : sessionStorage;
    const raw = storage.getItem(STORAGE_KEY);
    if (raw) {
      try {
        const parsed = JSON.parse(raw) as StoredAuth;
        storage.setItem(STORAGE_KEY, JSON.stringify({ ...parsed, user }));
      } catch {
        // เก็บ persisted state ไม่ได้ก็ไม่เป็นไร — state ใน memory ยังอัปเดตด้านล่างอยู่ดี
      }
    }
    set({ user });
  },
  logout: () => {
    localStorage.removeItem(STORAGE_KEY);
    sessionStorage.removeItem(STORAGE_KEY);
    set({ token: null, user: null });
  },
}));
