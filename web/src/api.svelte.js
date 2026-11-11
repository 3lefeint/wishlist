// Calls return { kind, status, data, message }:
//   kind 'ok'      - success (data holds the parsed JSON, null for 204)
//   kind 'session' - Cloudflare Access redirected to its login, or answered 401
//   kind 'http'    - the server refused (status, message)
//   kind 'error'   - the server could not be reached
export const net = $state({ sessionExpired: false });

export async function request(method, path, body) {
  const init = {
    method,
    redirect: 'manual', // makes an Access login redirect visible as an opaque response
    headers: { Accept: 'application/json' },
  };
  if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  let res;
  try {
    res = await fetch(path, init);
  } catch {
    // A cross-origin redirect to the Access login without CORS headers also lands here.
    net.sessionExpired = true;
    return { kind: 'session' };
  }
  if (res.type === 'opaqueredirect' || res.status === 401) {
    net.sessionExpired = true;
    return { kind: 'session' };
  }
  if (res.status >= 500 && res.status !== 503) return { kind: 'error', status: res.status };
  const text = await res.text();
  if (!res.ok) return { kind: 'http', status: res.status, message: text.trim() };
  if (res.status === 204 || text === '') return { kind: 'ok', status: res.status, data: null };
  try {
    return { kind: 'ok', status: res.status, data: JSON.parse(text) };
  } catch {
    // HTML instead of JSON: a login page answered in place of the API.
    net.sessionExpired = true;
    return { kind: 'session' };
  }
}

export const api = {
  get: (path) => request('GET', path),
  post: (path, body = {}) => request('POST', path, body),
  put: (path, body = {}) => request('PUT', path, body),
  del: (path) => request('DELETE', path),
};

// Uploads an image for a gift (multipart); same result kinds as request().
export async function uploadImage(giftId, file) {
  const form = new FormData();
  form.append('image', file);
  let res;
  try {
    res = await fetch(`/api/gifts/${giftId}/image`, { method: 'POST', body: form, redirect: 'manual' });
  } catch {
    net.sessionExpired = true;
    return { kind: 'session' };
  }
  if (res.type === 'opaqueredirect' || res.status === 401) {
    net.sessionExpired = true;
    return { kind: 'session' };
  }
  if (res.ok) return { kind: 'ok', status: res.status, data: null };
  return { kind: 'http', status: res.status, message: (await res.text()).trim() };
}
