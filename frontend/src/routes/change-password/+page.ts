import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const ssr = false;

// Standalone guard: a valid token is required to reach this page, but unlike the
// dashboard guard it does NOT bounce must_change_password users away — this is
// exactly where they are supposed to land. No token at all → back to login.
export const load: PageLoad = async () => {
  if (typeof localStorage === 'undefined') return {};
  const token = localStorage.getItem('lintasan_token');
  if (!token) {
    throw redirect(302, '/login');
  }
  return {};
};
