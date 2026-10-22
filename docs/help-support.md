# Help and support

Core owns authenticated support receipt and notification. Apply migration 42 before
starting the new server. `support_messages` is a private durable outbox, separate
from tenant-editable email queues. No SaaS control-plane service connection is required.

## Deployment configuration

The default `help.provider: none` exposes only the configured `server.docs.url`
(bundled `/docs/` when blank). Loading/opening Help makes no commercial support
requests. No getting-started action is shown because no separate destination exists.

Self-hosted operators may set `help.provider: operator`, `help.operator_url` to an
HTTP(S) URL or root-relative path, and `help.operator_label` to an explicit label
such as “Contact your portal operator”. This is a link, not tenant/client messaging.

Managed ClientShare deployments set:

```yaml
tenancy:
  mode: hosted
help:
  provider: clientshare
  recipient: support@example.com # replace with the monitored ClientShare mailbox
  booking_url: "" # optional reusable public booking URL
email:
  enabled: true
  smtp: # configure existing host, port, username, password and from fields
```

Only use a reusable public booking URL. Never configure a private customer
invitation. This feature does not create calendar resources.

## Authorization and boundaries

`GET /api/help` returns per-user capabilities; `POST /api/help/messages` requires
authenticated hosted administrators/managers. Existing authentication binds the
JWT to the resolved tenant. The handler reloads the user in that tenant, enforcing
the current database role as well as role middleware. Client users and secure-link
recipients cannot submit. Anonymous requests are rejected by authentication.
Recipient and SMTP credentials are never public capabilities.

The payload is strictly `id` (UUID v4 retry key), `message` (1–8000 bytes), and
`screen` (allowlisted section name). Unknown fields, including identity overrides,
are rejected. Identity, role, email, tenant, provider, timestamp and version come
from the server. Route IDs, queries, secure-link tokens, files and diagnostics are
not attached. Ten requests per account/tenant per hour are allowed per server
process; use shared edge limits for additional multi-replica abuse protection.

## Receipt and operations

202 means committed durable receipt, not successful SMTP delivery. Retries using
the same key and message return the original receipt; changed content with that
key returns 409. Keys are scoped to tenant and requester. The UI retains the key
on transport failure and prevents simultaneous submission. Drafts stay in memory
while the component remains mounted; reload/sign-out clears them.

A worker checks every 30 seconds, leasing up to 20 rows for ten minutes. SMTP
failures remain queued; inspect `attempts`, `next_attempt_at` and `notified_at` for
delivery health. Receipt IDs appear in notification subjects, with reply identity
in the body. SMTP delivery is at-least-once: a crash after SMTP acceptance can
duplicate a notification, not the support receipt. Back up the database and include
this table in your support-data retention policy. Disabling the provider stops
delivery and preserves pending receipts.

The SaaS checkout inspected at ff8c62b contains a durable marketing receiver, but
its public payload has no authenticated tenant/requester contract. This release
reuses its idempotency and durable-outbox conventions without treating public
marketing input as trusted tenant identity or coupling Core to a second service.
