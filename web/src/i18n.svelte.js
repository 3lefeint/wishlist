const messages = {
  en: {
    appName: 'Wishlist',
    loading: 'Loading…',
    signedInAs: 'Signed in as {email}',
    pendingTitle: 'Waiting for approval',
    pendingText: 'Your registration was received. An administrator has to approve it before you can use Wishlist.',
    blockedTitle: 'Access blocked',
    blockedText: 'Your account was blocked. Please contact an administrator.',
    sessionTitle: 'Session expired',
    sessionText: 'Please sign in again.',
    reload: 'Sign in again',
    errorTitle: 'Something went wrong',
    errorText: 'The server could not be reached. Try again in a moment.',
    retry: 'Retry',
    closedTitle: 'Registration temporarily closed',
    closedText: 'Too many registrations are waiting for approval. Please try again later.',
    welcome: 'Welcome',
    admin: 'Administrator',
    language: 'Language',
  },
  de: {
    appName: 'Wunschliste',
    loading: 'Lädt…',
    signedInAs: 'Angemeldet als {email}',
    pendingTitle: 'Warte auf Freigabe',
    pendingText: 'Deine Registrierung ist eingegangen. Ein Administrator muss sie freigeben, bevor du die Wunschliste nutzen kannst.',
    blockedTitle: 'Zugang gesperrt',
    blockedText: 'Dein Konto wurde gesperrt. Bitte melde dich bei einem Administrator.',
    sessionTitle: 'Sitzung abgelaufen',
    sessionText: 'Bitte melde dich erneut an.',
    reload: 'Erneut anmelden',
    errorTitle: 'Etwas ist schiefgelaufen',
    errorText: 'Der Server ist nicht erreichbar. Versuche es gleich noch einmal.',
    retry: 'Erneut versuchen',
    closedTitle: 'Registrierung vorübergehend geschlossen',
    closedText: 'Zu viele Registrierungen warten auf Freigabe. Bitte versuche es später noch einmal.',
    welcome: 'Willkommen',
    admin: 'Administrator',
    language: 'Sprache',
  },
};

export const languages = Object.keys(messages);

function initial() {
  try {
    const saved = localStorage.getItem('lang');
    if (saved && messages[saved]) return saved;
  } catch {}
  return navigator.language?.toLowerCase().startsWith('de') ? 'de' : 'en';
}

export const i18n = $state({ lang: initial() });

export function setLang(lang) {
  if (!messages[lang]) return;
  i18n.lang = lang;
  document.documentElement.lang = lang;
  try {
    localStorage.setItem('lang', lang);
  } catch {}
}

export function t(key, params = {}) {
  const text = messages[i18n.lang][key] ?? messages.en[key] ?? key;
  return text.replace(/\{(\w+)\}/g, (_, k) => params[k] ?? '');
}
