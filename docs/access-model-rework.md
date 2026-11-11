# Roles and access implementation plan

Status: implementation in progress. This document describes the target model,
not currently available functionality.

## Contract

- User type is Internal or External and does not grant document access.
- Internal organization roles initially are Member and Administrator. External
  people have no organization administration role.
- Access roles belong to a tenant and are assigned at workspace, folder or file
  scope. Names are editable; stable IDs identify roles.
- Permissions distinguish reading, uploading, deleting, inviting external
  recipients, creating sharing links and managing other people's access.
- Groups collect users, not permissions. A group receives a role at a scope;
  adding a member gives that member the group's existing assignments. No nesting.
- Tenant policy and the assigning actor's authority constrain every assignment.
  A role edit is also an access change and must obey the same ceiling.
- Inheritance remains additive. Removing one assignment cannot revoke another.
- Role permission edits update existing assignments. Archive prevents new
  assignments without revoking existing ones. Deletion requires replacement.
- Administrative authority and all-workspace access are explicit and separate
  from scoped document permissions. Preserve existing administrator behavior.

## Starting access roles

| Role | Read | Upload | Delete | Invite external | Create links | Manage access |
| --- | --- | --- | --- | --- | --- | --- |
| Viewer | yes | no | no | no | no | no |
| Uploader | no | yes | no | no | no | no |
| Contributor | yes | yes | no | no | no | no |
| Client contact | yes | yes | no | yes | no | no |
| File manager | yes | yes | yes | no | no | no |
| Workspace manager | yes | yes | yes | yes | yes | yes |

Workspace manager is internal-only by default. Uploader/Contributor/Client
contact are not single-file roles. Existing permission combinations that do not
match these templates receive named legacy roles without losing permissions.

## Delivery sequence

1. **Domain and migration:** additive schema, tenant-owned templates, exact legacy
   permission conversion, identity classification and explicit organization-wide
   entitlements. Preserve original authorization fields until cutover.
2. **Authorization:** role-backed resolution for direct/group grants at every
   scope; separate invitation/link/access-management authority; tenant policy,
   quotas, notifications, listings and all-client access. Test live role edits,
   revocation, group changes, ceilings and cross-tenant isolation.
3. **Management APIs:** audited role/group CRUD with optimistic concurrency,
   impact previews and confirmation, archive/replacement, current-actor checks.
4. **User creation and sharing:** Internal/External, organization role, per-client
   access roles, optional groups; tenant-owned applicable role dropdowns;
   source-aware inherited access. Update user editing and people lists too.
5. **Verification and documentation:** migration rehearsal, backend/frontend
   suites, browser workflows and self-hosting guide. Remove legacy authorization
   dependence only after coverage demonstrates equivalent access.

Do not enable a partially migrated resolver or show controls backed only by the
old four permission flags. In particular, `can_share` must not silently stand in
for the three separate new sharing permissions.

## Current checkpoint

The additive foundation migration creates profiles, role templates, groups and
role assignments, and snapshots existing memberships/grants into the new model.
It does **not** change the active resolver, account creation, invitations or UI.
Original authorization records remain the source of truth. No management API
may edit the shadow assignments until cutover: subsequent legacy changes must
be reconciled before activation, and new tenants/users must receive their new
records through the corresponding creation transactions.

The permission conversion preserves the old sharing flags as candidate role
permissions, not as a substitute for ownership/provenance rules. Cutover tests
must retain current restrictions on which recipients and invitations a person
can manage, including legacy external sharing. Pending invitations and links
are deliberately left unchanged until their lifecycle is migrated atomically.
