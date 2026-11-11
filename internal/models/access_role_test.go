package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultAccessRolesAreIndependentPermissionBundles(t *testing.T) {
	roles := DefaultAccessRoles("tenant")
	require.Len(t, roles, 6)
	for _, role := range roles {
		require.Equal(t, "tenant", role.TenantID)
		require.NotNil(t, role.TemplateKey)
		require.True(t, role.AllowWorkspace)
		require.True(t, role.AllowFolder)
	}
	contact, files, manager := roles[3], roles[4], roles[5]
	require.True(t, contact.CanInviteExternal)
	require.False(t, contact.CanCreateLinks || contact.CanManageAccess || contact.CanDelete)
	require.True(t, files.CanDelete)
	require.False(t, files.CanInviteExternal || files.CanCreateLinks || files.CanManageAccess)
	require.Equal(t, "internal", manager.Audience)
	require.False(t, manager.AppliesTo("external", "workspace"))
	require.True(t, manager.AppliesTo("internal", "workspace"))
	require.True(t, roles[0].AppliesTo("external", "file"))
	for _, role := range roles[1:4] {
		require.False(t, role.AppliesTo("external", "file"))
	}
	permissions, applicable := files.PermissionsAt("file")
	require.True(t, applicable)
	require.True(t, files.AppliesTo("external", "file"))
	require.True(t, permissions.CanRead && permissions.CanDelete)
	require.False(t, permissions.CanUpload)
	require.True(t, files.CanUpload, "projection must not mutate the role")
}

func TestAccessRoleRenamingDoesNotChangeEligibilityOrPermissions(t *testing.T) {
	role := DefaultAccessRoles("tenant")[0]
	before := role.RolePermissions
	role.Name = "Outside reviewer"
	require.Equal(t, before, role.RolePermissions)
	require.True(t, role.AppliesTo("external", "file"))
	require.False(t, role.AppliesTo("unknown", "file"))
	require.False(t, role.AppliesTo("external", "unknown"))
	now := time.Now()
	role.ArchivedAt = &now
	require.False(t, role.AppliesTo("external", "file"), "archival prevents new assignments")
}

func TestRolePermissionCeilingChecksEveryIndependentRight(t *testing.T) {
	full := RolePermissions{true, true, true, true, true, true}
	for _, permission := range []RolePermissions{
		{CanRead: true}, {CanUpload: true}, {CanDelete: true}, {CanInviteExternal: true}, {CanCreateLinks: true}, {CanManageAccess: true},
	} {
		require.True(t, permission.Within(full))
		require.False(t, permission.Within(RolePermissions{}))
	}
	require.False(t, RolePermissions{CanCreateLinks: true}.Within(RolePermissions{CanInviteExternal: true}))
	require.False(t, RolePermissions{CanManageAccess: true}.Within(RolePermissions{CanCreateLinks: true}))
	require.True(t, (RolePermissions{}).Within(RolePermissions{}))
}
