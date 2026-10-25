import { writable } from 'svelte/store';

export type ConfirmTone = 'danger' | 'default';

export type ConfirmOptions = {
  title: string;
  message?: string;
  detail?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  tone?: ConfirmTone;
};

export type ConfirmRequest = {
  id: number;
  title: string;
  message: string;
  detail: string;
  confirmLabel: string;
  cancelLabel: string;
  tone: ConfirmTone;
  resolve: (value: boolean) => void;
};

export const confirmRequest = writable<ConfirmRequest | null>(null);

let sequence = 0;
let hostMounted = false;

/** The dashboard layout mounts one host; tests or standalone pages fall back
 * to the native dialog so an action can never hang waiting for a host. */
export function setConfirmHostMounted(mounted: boolean) {
  hostMounted = mounted;
}

export function confirmAction(options: ConfirmOptions): Promise<boolean> {
  const title = options.title?.trim() || 'Are you sure?';
  const message = options.message?.trim() || '';
  const detail = options.detail?.trim() || '';

  if (!hostMounted || typeof window === 'undefined') {
    const text = [title, message, detail].filter(Boolean).join('\n\n');
    return Promise.resolve(window.confirm(text));
  }

  return new Promise((resolve) => {
    confirmRequest.update((current) => {
      current?.resolve(false);
      return {
        id: ++sequence,
        title,
        message,
        detail,
        confirmLabel: options.confirmLabel?.trim() || 'Confirm',
        cancelLabel: options.cancelLabel?.trim() || 'Cancel',
        tone: options.tone ?? 'default',
        resolve
      };
    });
  });
}

export function resolveConfirm(value: boolean) {
  confirmRequest.update((current) => {
    current?.resolve(value);
    return null;
  });
}
