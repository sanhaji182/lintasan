import { writable } from 'svelte/store';

export const page = {
  url: new URL('http://localhost/dashboard/connections'),
  subscribe: writable({ url: new URL('http://localhost/dashboard/connections') }).subscribe,
};
