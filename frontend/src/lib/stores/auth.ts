import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export interface AuthUser {
  id: string;
  username: string;
  role: string;
}

function createAuthStore() {
  const { subscribe, set } = writable<{
    token: string | null;
    user: AuthUser | null;
    isAuthenticated: boolean;
  }>(getInitialState());

  return {
    subscribe,

    login(token: string, user: AuthUser) {
      if (browser) {
        localStorage.setItem('lintasan_token', token);
        localStorage.setItem('lintasan_user', JSON.stringify(user));
      }
      set({ token, user, isAuthenticated: true });
    },

    logout() {
      if (browser) {
        localStorage.removeItem('lintasan_token');
        localStorage.removeItem('lintasan_user');
      }
      set({ token: null, user: null, isAuthenticated: false });
    },

    getToken(): string | null {
      if (browser) return localStorage.getItem('lintasan_token');
      return null;
    },

    restore() {
      if (!browser) return;
      const token = localStorage.getItem('lintasan_token');
      const userStr = localStorage.getItem('lintasan_user');
      if (token && userStr) {
        try {
          const user = JSON.parse(userStr) as AuthUser;
          set({ token, user, isAuthenticated: true });
        } catch {
          this.logout();
        }
      }
    }
  };
}

function getInitialState() {
  if (!browser) return { token: null, user: null, isAuthenticated: false };
  const token = localStorage.getItem('lintasan_token');
  const userStr = localStorage.getItem('lintasan_user');
  if (token && userStr) {
    try {
      const user = JSON.parse(userStr) as AuthUser;
      return { token, user, isAuthenticated: true };
    } catch {
      return { token: null, user: null, isAuthenticated: false };
    }
  }
  return { token: null, user: null, isAuthenticated: false };
}

export const auth = createAuthStore();

// Initialize on module load
if (browser) {
  auth.restore();
}
