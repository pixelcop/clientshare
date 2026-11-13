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

Identity fields are read after request handling. They are omitted when unknown;
public/static routes do not perform extra authentication or database lookups just
for logging. A failed login does not identify the user from its submitted email,
and a missing/invalid session does not identify the user from its cookie.

Access logs do not include session-cookie presence or values, request bodies, or
authorization headers. Query strings and secure-link bearer path segments are
redacted. Restrict access to logs and set retention appropriate for IP addresses,
user agents, and tenant/user identifiers.
