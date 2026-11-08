# Secure link creation and email retries

These endpoints currently retain the administrator-only management gate. A
bearer-link visitor cannot create, email, inspect management history or revoke
links. Each operation is tenant-scoped. These are client-wide links, not item
sharing endpoints.

## Create

`POST /api/clients/:id/links` accepts the existing `access_type`, `expiry_days`
and optional legacy `email` fields. `read`, legacy `view`, `write` and
`readwrite` retain their stored meanings. Upload does not imply deletion.

New callers should send a random, printable 8–128-character `Idempotency-Key`
and may include an exact RFC 3339 `expires_at` within the supported maximum
lifetime. The interactive form displays and submits that exact timestamp.
Without it, legacy callers retain expiry-days behavior.

Repeating a key with the same normalized request returns the original link,
token and expiry. Reusing it with different settings returns `409`. Keys are
hashed with tenant, actor and operation/target scope; raw keys and bearer
tokens are not added to retry records. Nullable unique creation keys preserve
legacy links. A revoked/expired request never creates a replacement under the
same key; it returns the original, unavailable link.

New UI flows create without emailing, then retain the result for Copy or Email.
The legacy `email_queued` response means queue acceptance, not recipient
delivery. New UI email actions use the separate endpoint below.

## Email an existing link

`POST /api/links/:id/email` requires an `Idempotency-Key` and JSON
`{"email":"recipient@example.com"}`. It never creates a link or changes its
scope, permissions or expiry. Emailing is not identity verification or account
membership, and it does not subscribe the address to client upload updates.

| Response | Meaning |
| --- | --- |
| `202`, `status: queued` | Durable queue acceptance. Delivery is unconfirmed. |
| `202`, `status: sent` | A replay found the same message already accepted by SMTP. Delivery/read status remains unconfirmed. |
| `503`, `status: unavailable` | Email is not configured/available; copy the existing URL or retry later. |
| `503`, `status: failed` | Queueing failed; retry the same key and recipient. |
| `409` | Key was already used for a different recipient/link. |
| `410` | The authorized link is expired or revoked. |
| `404` | Missing or wrong-tenant link. |

Deduplication is enforced by a nullable unique database key and survives
process restarts and concurrent retries. Retain the key after a lost response;
use a new key only for a genuinely new send. The queue does not mark mail sent
before SMTP succeeds. As with SMTP generally, an uncertain SMTP acknowledgement
can cause a later transport retry; this API guarantees deduplicated enqueue,
not exactly-once recipient delivery.

## History, revocation and upgrade

Migration 049 adds revocation timestamps and retry fields without changing
existing tokens, URLs, access modes or expiry values. Revocations are retained
instead of deleting rows. Link management lists return metadata but no usable
URL/token for expired/revoked entries. Previously deleted history cannot be
reconstructed.

Current sessions, bootstrap, inspection and registration reject revoked links.
Revocation cancels unsent associated queue records; dispatch also rechecks link
and client validity. Email already handed to SMTP cannot be recalled. Other
links and account memberships remain unchanged.

Back up the database before upgrading. Do not run an older binary that ignores
`revoked_at`: retained revoked rows could become usable again. Use a forward
corrective build, or explicitly neutralize incompatible access paths before
any downgrade. Do not remove retry records while clients may still retry them.
