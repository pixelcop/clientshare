const commentHash = /^#comment-([0-9A-HJKMNP-TV-Z]{26})$/i;
const commentPath = /^\/files\/[0-9A-HJKMNP-TV-Z]{26}#comment-[0-9A-HJKMNP-TV-Z]{26}$/i;
const handoffKey = 'clientshare:comment-login-return';

export function commentIdFromHash(hash: string) {
  return commentHash.exec(hash)?.[1]?.toUpperCase();
}

// Hosted passkey sign-in leaves this origin, then returns to /login with its
// own fragment. Keep the intended file link in the current browser tab.
export function rememberCommentLogin(path: unknown) {
  try {
    sessionStorage.removeItem(handoffKey);
    if (typeof path === 'string' && commentPath.test(path)) {
      sessionStorage.setItem(handoffKey, JSON.stringify({ path, expires: Date.now() + 900_000 }));
    }
  } catch {
    // Sign-in still works when storage is unavailable.
  }
}

export function takeCommentLogin(): string {
  try {
    const stored = sessionStorage.getItem(handoffKey);
    sessionStorage.removeItem(handoffKey);
    if (stored) {
      const { path, expires } = JSON.parse(stored);
      if (typeof path === 'string' && commentPath.test(path) && expires > Date.now()) {
        return path;
      }
    }
  } catch {
    // Sign-in still works when storage is unavailable.
  }
  return '';
}
