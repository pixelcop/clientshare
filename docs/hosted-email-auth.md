# Hosted email authentication

Trusted hosted control planes can request email authentication through the
internal API. This does not enable magic links on the self-hosted portal login
form or change recipient secure-link permissions.

Migration 48 creates `hosted_email_tokens`. Apply migrations before calling:

- `POST /internal/auth/hosted-email/request`: `email`, `purpose` (`login` or
  `reset`), `destination` (`portal` or `account`). Returns a delivery email and
  raw token only to the authenticated internal caller, or an empty object when
  delivery should be suppressed. The control plane must send the email and must
  never return these fields to a public request caller.
- `POST /internal/auth/hosted-email/complete`: `token`, `purpose`, and `password`
  for a reset. Returns the token-bound `destination` and, for login, the existing
  hosted login result (including the private account session and portal handoff).
  The control plane must keep account sessions in HttpOnly cookies, not expose
  them in browser JSON or URLs, and independently enforce billing ownership.

Tokens are 256-bit random values, stored as purpose-separated HMAC hashes,
expire in 15 minutes, and are consumed transactionally once. A per-user/purpose
atomic 60-second cooldown prevents concurrent resends; a later request replaces
the old link. Unknown, ambiguous and inactive identities do not receive tokens.
Completion revalidates identity, tenant availability and current credentials.
Password reset revokes sessions, handoffs and outstanding recovery/sign-in links.

The hosted passkey-options response includes `password_available` independently
of passkey enrollment, including when hosted passkey ceremonies are disabled.
It reflects a usable password hash on the Core user, not a provisioning record.
Control planes can therefore keep passkeys primary and show a password field
after email fallback only for password-backed identities. This pre-authentication
method metadata, like passkey availability, can reveal supported authentication
methods; rate-limit public access and do not describe it as enumeration-proof.

Public integrations must enforce browser-origin protection and IP rate limits,
return generic request responses, generate URLs from trusted configuration, and
avoid logging email tokens or credentials. Use URL fragments and an explicit
POST confirmation so mail scanners cannot consume links through GET requests.
Never allow a caller to choose a completion redirect URL. Password resets do
not automatically authenticate the user.
