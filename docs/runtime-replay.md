# Targeted runtime replays

Core exposes an anonymous-safe `GET /api/config` endpoint. Its
`debug.replay` object contains `enabled`, `start_on_failure` and, for an active
match, `expires_at`. Responses are private and never cached by HTTP clients.
No targeting conditions, emails, credentials or operator labels are returned.

Replay requests are stored in Core. A control plane using the existing internal
API credential can list/create requests at `/internal/replay-requests` and revoke
one with `DELETE /internal/replay-requests/:id`. There is no public management
route. Apply migration 59 before enabling management.

Example creation body:

```json
{
  "label": "Investigate portal sign-in",
  "targets": [
    { "ip": "192.0.2.0/24", "tenant_id": "TENANT_ID" },
    { "user_email": "person@example.invalid" }
  ],
  "start_on_failure": false,
  "expires_at": "<RFC3339 timestamp within the next 24 hours>"
}
```

Conditions within each group are ANDed; groups and requests are ORed. Empty
groups are rejected. IP addresses and CIDRs are supported, hostnames are exact
case-insensitive DNS names, and identity IDs are exact. Email matching is
case-insensitive and requires a verified, tenant-local portal session. Submitted
emails, query parameters and identity headers never establish identity. IP
matching uses the same `utils.GetIP(c)` helper as access logs, including a cached
request IP, the first non-empty IP in Fiber's client IP list, and a remote-IP
fallback. Fiber's client IP follows the application's trusted-proxy policy;
ensure the origin is restricted to trusted ingress in your deployment.

The authenticated internal `GET /internal/replay-targets` directory supports
`kind=tenants|users`, a literal `q` search, and optional `tenant_id` for users.
It returns at most 20 ID/name/email suggestions, without credentials. Request
creation validates tenant/user IDs against the current directory and rejects
users outside a specified tenant. Email selectors may name an address not yet
in the directory.

At most 100 active requests and 10 groups per request are accepted. The bounded
active-rule cache refreshes every five seconds, including when empty. Local
management invalidates the cache immediately; other API replicas refresh within
five seconds. Expiry is also checked on every match. Database failures disable
collection rather than returning stale eligibility.

The portal checks config once at startup, after subsequent session/identity
changes, every 30 seconds, and on back/forward-cache page restoration. Concurrent
refreshes share one request; the initial authentication check does not repeat
the startup config request. Eligible sessions start Sentry Replay
manually; general replay sampling remains off. Revocation normally propagates
within approximately 35 seconds in active tabs. Background tab throttling or
offline clients may delay polling, so requests always have an explicit expiry.
An unsuccessful config check disables recording. Browser expiry timers stop
recording independently of a successful refresh. Already uploaded replays are
not deleted by revocation.

`start_on_failure` means start **after** the first same-origin `/api/` response
with a 4xx/5xx status, from either Axios or fetch. `/api/config` itself is excluded.
There is no pre-error recording/buffering. A signed-out `/auth/me` 401 counts as a
failure. Immediate recording takes precedence when multiple requests match;
the earliest matching expiry forces a fresh evaluation.

Replays require a portal build with `VITE_SENTRY_DSN` set, a reachable Sentry
ingestion endpoint and a compatible CSP (including Replay's worker). With no
DSN, config is still fetched but nothing is uploaded. Static UI text (navigation,
buttons, labels and instructions) stays readable. Client/workspace names, user
names/emails/initials, file and folder names/paths, file sizes/timestamps, activity
details, email subjects/recipients, tenant branding text and data-bearing
notifications are masked on their shared UI surfaces. All form values remain
masked. Media, iframes and canvases are blocked; PDF and plain-text document
contents are explicitly blocked as well. Additional
recording events (console/network payloads) are dropped and network body capture
is disabled. Identifying text attributes (including titles and accessibility
labels) are masked. Links with query strings, fragments, secure-link paths or
contact destinations are blocked because URL attributes are normalized by the
recorder before attribute masking. Ordinary route links remain visible; replay event URLs omit query
strings/fragments, and recording is deferred on credential-bearing bootstrap
URLs (such as an invitation token or hosted-login handoff) until consumed.
When adding UI that renders identities, document metadata or other private
values, mark the containing DOM element with `data-sentry-mask`. Use
`data-sentry-block` for document content. Teleported dropdowns/dialogs need
masking on the overlay/title itself, not just on a parent outside the teleport.
Review your Sentry data-processing/consent requirements and DOM
attributes before enabling recordings for real users; selective masking requires
reviewing new data-bearing UI and is not a substitute
for a privacy review. Verified user IDs and tenant IDs identify Sentry events;
email is not explicitly added to Sentry user metadata.
