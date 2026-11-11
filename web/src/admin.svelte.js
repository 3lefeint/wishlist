import { api } from './api.svelte.js';

// Number of registrations waiting for approval, shown as a badge for administrators.
export const admin = $state({ pending: 0 });

export async function refreshPending() {
  const r = await api.get('/api/admin/users?status=pending');
  if (r.kind === 'ok') admin.pending = r.data.length;
}
