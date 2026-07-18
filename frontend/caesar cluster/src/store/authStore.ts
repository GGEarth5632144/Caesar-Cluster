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
  logout: () => {
    localStorage.removeItem(STORAGE_KEY);
    sessionStorage.removeItem(STORAGE_KEY);
    set({ token: null, user: null });
  },
}));
