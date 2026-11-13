# Access logs

Request completion logs include the existing `requestId`, request/response fields,
and timing. Additional context includes:

- `req.hostname`: the request hostname (not the server machine's hostname).
- `req.userAgent`: the caller's User-Agent, limited to 1,024 bytes. This is
  untrusted client-provided information, not a verified browser identity.
- `commit_hash`: the build's injected Git commit, or `dev` when unavailable.
- `tenant_id`: the resolved tenant, when available.
- `user_id`: the verified user, when available, including successful user-session
  issuance on login routes.
- `auth_outcome`: an authentication result, only when an auth check or supported
  authentication operation ran. It is independent of the final HTTP status: a
  valid session may still get a permission-denied or application-error response.

## Authentication outcomes

Protected portal requests report `authenticated`, `missing_cookie`,
`invalid_session`, `validation_unavailable`, or `tenant_mismatch`.
`invalid_session` intentionally groups unknown, expired, revoked, wrong-tenant,
and otherwise invalid sessions: the existing validation query does not
distinguish these causes, and logging adds no extra session lookups.

Password login and passkey verification report `invalid_request`,
`invalid_credentials`, or `validation_unavailable` on failure. Hosted handoff
exchange also reports `invalid_handoff`. User-session issuance reports
`session_issued` or `session_issue_failed`; auth-route rate limits report
`rate_limited`. Logout reports `logout_completed` or `logout_failed`.
`logout_completed` includes idempotent logout without an existing session.
These outcomes do not expose submitted email addresses, tokens, or session IDs.

Public requests and operations not instrumented for authentication outcomes omit
the field; absence does not imply authentication success or failure.

## Identity and sensitive data

Identity fields are read after request handling. They are omitted when unknown;
public/static routes do not perform extra authentication or database lookups just
for logging. A failed login does not identify the user from its submitted email,
and a missing/invalid session does not identify the user from its cookie.

Access logs do not include session-cookie presence or values, request bodies, or
authorization headers. Query strings and secure-link bearer path segments are
redacted. Restrict access to logs and set retention appropriate for IP addresses,
user agents, and tenant/user identifiers.
