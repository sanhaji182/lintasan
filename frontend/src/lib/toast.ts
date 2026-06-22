import { writable } from 'svelte/store';

export interface ToastDetail {
  code?: string;
  type?: string;
  param?: string;
  message: string;
  hint?: string;
}

export interface ToastItem {
  id: number;
  message: string;
  type: 'success' | 'error' | 'info' | 'warning';
  detail?: ToastDetail;
  duration: number;
}

let nextId = 0;
const { subscribe, update } = writable<ToastItem[]>([]);

export const toasts = { subscribe };

export function showToast(
  message: string,
  type: 'success' | 'error' | 'info' | 'warning' = 'info',
  duration: number = 3000,
  detail?: ToastDetail,
) {
  const id = nextId++;
  update(t => [...t, { id, message, type, detail, duration }]);
  if (duration > 0) {
    setTimeout(() => {
      dismissToast(id);
    }, duration);
  }
}

export function dismissToast(id: number) {
  update(t => t.filter(item => item.id !== id));
}
