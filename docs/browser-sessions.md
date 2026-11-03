# Browser sessions

Core browser authentication uses a random 256-bit credential in the host-only,
Secure, HttpOnly `clientshare_session` cookie. Only its SHA-256 digest is stored.
JWT cookies and Authorization bearer tokens no longer authenticate browser routes.
Existing browsers must sign in again after cutover. Passkey credentials and
single-use invitation, password-reset, signup, and hosted-login tokens remain valid.
The existing JWT secret configuration is retained for other token-signing paths.

## Lifetime and revocation

- Portal sessions have a 30-day absolute lifetime and a 24-hour idle lifetime.
- Account sessions have a 24-hour absolute lifetime and a separate audience.
- Every authenticated request checks persisted expiry/revocation and current user
  or recipient-link permissions. Idle-expiry writes are throttled to once an hour.
- `POST /api/auth/logout` revokes the current portal session. `?scope=group`
  revokes its related account/portal sessions and pending handoffs; `?scope=all`
  revokes every session and pending authentication challenge for that tenant user.
- Password reset revokes all of the user's sessions, pending handoffs, and
  WebAuthn challenges. New passkey enrollment is bound to its initiating session.

Hosted login issues a separate account credential through the private API and a
two-minute, single-use handoff for the portal. Account credentials are intended for
trusted server-side callers. Changing portal identity revokes its previous session
group. Ordinary single-audience logout leaves the other audience intact.

Recipient links prompt before replacing an authenticated portal identity. The
confirmation is bound to the current session ID, so stale confirmation pages cannot
silently replace a newer sign-in. Same-origin mutations are checked using Origin
and Fetch Metadata. Tabs receive identity-change events and revalidate on focus.

## Migration

Apply migration 045 before starting the updated application. Existing JWT cookies
are not converted to opaque sessions; users must sign in again.
