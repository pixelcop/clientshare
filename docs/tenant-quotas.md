# Tenant quotas

Tenant quotas are an optional feature for limiting staff users and storage in multi-tenant or single-tenant deployments. Limits are stored in each tenant's `tenant_entitlements` row as `max_users` and `max_storage_bytes` and enforced locally. Apply migration **41** before running this version.

Set either field to a positive value to enable that quota, or zero to leave that resource unlimited. Limits can be populated through the internal tenant API or by an operator managing tenant entitlement records. In single-tenant mode, populate the default tenant's entitlement row. Enforcement uses the same rules in both tenancy modes and requires no external service.

- `max_users` counts admins and managers, including the initial owner and pending invitees. Client users do not consume seats. Resends and edits to existing staff do not consume seats. Promoting a client to staff does.
- Storage counts the actual bytes of live `files` rows (`type = 'file'`, `deleted_at IS NULL`) across the whole tenant. Folders and retained deleted files do not count. Physical deletion remains the cleanup worker's responsibility.
- Limits of zero mean unlimited in either tenancy mode. Staff and storage quotas can be enabled independently. Negative limits are rejected by the internal API. Missing entitlements fail closed.
- A lower limit never deletes data or users. New consumption is blocked, while existing staff edits, downloads and deletion continue to work.

Staff checks and mutations share a transaction. A write lock on the tenant entitlement row serializes concurrent requests across processes on SQLite and PostgreSQL.

Uploads reserve bytes before writing storage. Reservations are committed separately so storage I/O does not hold database locks. Finalization rechecks the current limit and commits measured bytes and reservation removal atomically. Failed uploads release reservations; interrupted reservations expire after 24 hours and are pruned on the next reservation. Finalization rejects expired reservations. All authenticated and public-link uploads use this path. Multi-file uploads are processed per file; files already accepted remain when a later file fails.

HTTP quota errors use status **409** with `code: staff_limit_reached` or `code: storage_limit_reached` and a user-facing `error` message.

Usage endpoints:

- `GET /api/settings/usage`: authenticated tenant admin; tenant comes from the session.
- `GET /internal/tenants/:id/usage`: caller authenticated through the internal API.

Both return `staff_users`, `max_users`, `storage_bytes`, `reserved_bytes`, and `max_storage_bytes`. The Settings page displays tenant usage and configured limits. Storage is displayed in decimal GB (1 GB = 1,000,000,000 bytes).
