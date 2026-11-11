// Result kinds of a call: 'ok', 'session' (Cloudflare Access redirected to its
// login, the session expired), 'closed' (registration closed), 'error'.
export async function getMe() {
  let res;
  try {
    // redirect: 'manual' makes an Access login redirect visible as an opaque response.
    res = await fetch('/api/me', { redirect: 'manual', headers: { Accept: 'application/json' } });
  } catch {
    // A cross-origin redirect to the Access login without CORS headers also lands here.
    return { kind: 'session' };
  }
  if (res.type === 'opaqueredirect' || res.status === 401 || res.status === 403) return { kind: 'session' };
  if (res.status === 503) return { kind: 'closed' };
  if (!res.ok) return { kind: 'error' };
  try {
    return { kind: 'ok', user: await res.json() };
  } catch {
    return { kind: 'session' };
  }
}
