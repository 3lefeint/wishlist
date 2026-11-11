function current() {
  return location.hash.replace(/^#/, '') || '/';
}

export const router = $state({ path: current() });

window.addEventListener('hashchange', () => {
  router.path = current();
});

export function navigate(path) {
  location.hash = path;
}

// match('/circles/:id', '/circles/12') -> { id: '12' }, or null
export function match(pattern, path) {
  const a = pattern.split('/');
  const b = path.split('/');
  if (a.length !== b.length) return null;
  const params = {};
  for (let i = 0; i < a.length; i++) {
    if (a[i].startsWith(':')) params[a[i].slice(1)] = decodeURIComponent(b[i]);
    else if (a[i] !== b[i]) return null;
  }
  return params;
}
