import { writable } from 'svelte/store';

// A toast can be a simple message OR a structured "detail" object that the
// renderer turns into a rich multi-line display. The structured form is used
// to surface OpenAI-standard error envelopes (code + type + param + message + hint)
// from the test endpoint.
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
  type: 'success' | 'error' | 'info';
  detail?: ToastDetail;
  duration: number;
}

let nextId = 0;
const { subscribe, update } = writable<ToastItem[]>([]);

export const toasts = { subscribe };

export function showToast(
  message: string,
  type: 'success' | 'error' | 'info' = 'info',
  duration: number = 3000,
  detail?: ToastDetail,
) {
  const id = nextId++;
  update(t => [...t, { id, message, type, detail, duration }]);
  setTimeout(() => {
    update(t => t.filter(item => item.id !== id));
  }, duration);
}
