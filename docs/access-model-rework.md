# Roles and access implementation plan

Status: role-backed authorization, management APIs and user/sharing interfaces
implemented. Legacy fields remain compatibility snapshots for older clients and
upgraded installations; they are not the source of a role-backed assignment's
permissions.

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

Migration 055 is additive. Migrations 056–057 attach tenant-local roles to live
memberships, grants and invitations, reconcile checkpoint-era changes, and remove
obsolete shadow snapshots. Direct grants retain their original identities,
versions, credentials, provenance and expiry. Group/additive grants use
`role_assignments`. A tenant catalog marker prevents deleted default roles from
being recreated. Role names and template keys never authorize operations.

Legacy sharing roles preserve ownership/provenance rules; changing their sharing
permissions explicitly switches to the independent sharing-rights contract.
Pending invitations reference roles while keeping credentials and lifecycle.
Independent accepted invitations remain separate assignments rather than
flattening scoped roles/expiries into stronger legacy capability bits. Link
authorization, legacy link routes, notifications and workspace lists use current
rights. Downgrade refuses independent sharing/group data that the older schema
cannot safely represent; replace/remove those assignments before downgrading.
