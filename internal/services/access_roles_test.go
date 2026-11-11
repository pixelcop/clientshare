package services

import (
	"context"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func roleFixture(t *testing.T) (accessFixture, *AccessRoleService, map[string]models.AccessRole) {
	t.Helper()
	f, _ := sharingFixture(t)
	require.NoError(t, f.db.AutoMigrate(&models.Tenant{}, &models.AccessProfile{}, &models.AccessRoleCatalog{}, &models.AccessGroup{}, &models.AccessGroupMember{}, &models.RoleAssignment{}))
	require.NoError(t, f.db.Create(&models.Tenant{ID: "tenant", Slug: "tenant", Name: "Tenant"}).Error)
	s := &AccessRoleService{DB: f.db}
	roles, err := s.Roles(context.Background(), "tenant", "admin", true)
	require.NoError(t, err)
	byKey := map[string]models.AccessRole{}
	for _, r := range roles {
		if r.TemplateKey != nil {
			byKey[*r.TemplateKey] = r
		}
	}
	return f, s, byKey
}

func TestTenantRolesSeparateSharingAuthorityAndUpdateLiveAssignments(t *testing.T) {
	f, s, roles := roleFixture(t)
	ctx := context.Background()
	contact := roles["client-contact"]
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id = 'member'").Updates(map[string]any{"role_id": contact.ID, "capabilities_set": true, "can_read": true, "can_upload": true, "can_share": true}).Error)
	f.check(t, f.req("member", "", AccessInviteClient), true)
	f.check(t, f.req("member", "", AccessCreateLink), false)
	f.check(t, f.req("member", "folder", AccessManageAccess), false)
	f.check(t, f.req("member", "file", AccessDelete), false)
	sharing := &SharingService{DB: f.db, TokenSecret: "secret", LinkSigningKey: "secret"}
	_, err := sharing.CreateInvitation(ctx, f.req("member", "folder", AccessShare), InvitationInput{Email: "outside@example.test", RoleID: roles["viewer"].ID}, "one")
	require.NoError(t, err)
	_, err = sharing.CreateInvitation(ctx, f.req("member", "folder", AccessShare), InvitationInput{Email: "admin@example.test", RoleID: roles["viewer"].ID}, "two")
	require.ErrorIs(t, err, ErrSharingCapabilities)
	_, err = sharing.CreateLink(ctx, f.req("member", "folder", AccessShare), LinkInput{AccessType: "read", ExpiryDays: 7}, "three")
	require.Error(t, err)
	impact, err := s.Impact(ctx, "tenant", "admin", contact.ID)
	require.NoError(t, err)
	contact.Name = "Client coordinator"
	contact.CanDelete = true
	updated, err := s.SaveRole(ctx, "tenant", "admin", contact.ID, contact, contact.Version, impact.Fingerprint)
	require.NoError(t, err)
	require.Equal(t, contact.ID, updated.ID)
	f.check(t, f.req("member", "file", AccessDelete), true)
	_, err = s.SaveRole(ctx, "tenant", "admin", contact.ID, contact, contact.Version, impact.Fingerprint)
	require.ErrorIs(t, err, ErrRoleConflict)
	_, err = s.SaveRole(ctx, "tenant", "member", "", contact, 0, "")
	require.ErrorIs(t, err, ErrRoleForbidden)
	_, err = s.Roles(ctx, "foreign", "admin", false)
	require.Error(t, err)
}

func TestGroupAccessIsScopedAdditiveAndRevokedOnMembershipChange(t *testing.T) {
	f, s, roles := roleFixture(t)
	ctx := context.Background()
	input := GroupInput{Name: "Accounting team", MemberIDs: []string{"recipient"}, Assignments: []GroupAssignmentInput{{ClientID: "client", RoleID: roles["contributor"].ID}}}
	impact, err := s.PreviewGroup(ctx, "tenant", "admin", "", input)
	require.NoError(t, err)
	input.Impact = impact.Fingerprint
	group, err := s.SaveGroup(ctx, "tenant", "admin", "", input)
	require.NoError(t, err)
	f.check(t, f.req("recipient", "file", AccessRead), true)
	f.check(t, f.req("recipient", "folder", AccessUpload), true)
	f.check(t, f.req("recipient", "file", AccessDelete), false)
	f.check(t, f.req("recipient", "", AccessInviteClient), false)
	q, err := f.s.ClientScope(ctx, "tenant", "recipient", AccessContext)
	require.NoError(t, err)
	var ids []string
	require.NoError(t, q.Pluck("clients.id", &ids).Error)
	require.Equal(t, []string{"client"}, ids)
	f.check(t, AccessRequest{TenantID: "tenant", ClientID: "other-client", FileID: "other-file", Actor: AccessActor{UserID: "recipient"}, Operation: AccessRead}, false)
	// A direct file grant survives removing the independent group route.
	f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true})
	input.Version = group.Version
	input.MemberIDs = []string{}
	impact, err = s.PreviewGroup(ctx, "tenant", "admin", group.ID, input)
	require.NoError(t, err)
	input.Impact = impact.Fingerprint
	_, err = s.SaveGroup(ctx, "tenant", "admin", group.ID, input)
	require.NoError(t, err)
	f.check(t, f.req("recipient", "file", AccessRead), true)
	f.check(t, f.req("recipient", "sibling", AccessRead), false)
	f.check(t, f.req("recipient", "folder", AccessUpload), false)
	// Internal-only roles cannot be laundered through a group.
	input.Version = group.Version + 1
	input.MemberIDs = []string{"recipient"}
	input.Assignments[0].RoleID = roles["workspace-manager"].ID
	_, err = s.PreviewGroup(ctx, "tenant", "admin", group.ID, input)
	require.ErrorIs(t, err, ErrRoleInput)
}

func TestRoleArchiveDeleteReplacementAndNoDefaultResurrection(t *testing.T) {
	f, s, roles := roleFixture(t)
	ctx := context.Background()
	viewer := roles["viewer"]
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id='member'").Updates(map[string]any{"role_id": viewer.ID, "capabilities_set": true, "can_read": true, "can_upload": false, "can_share": false}).Error)
	impact, err := s.Impact(ctx, "tenant", "admin", viewer.ID)
	require.NoError(t, err)
	require.NoError(t, s.ArchiveRole(ctx, "tenant", "admin", viewer.ID, viewer.Version, impact.Fingerprint))
	f.check(t, f.req("member", "file", AccessRead), true)
	_, _, err = AssignmentRole(f.db, "tenant", viewer.ID, "external", "workspace")
	require.ErrorIs(t, err, ErrRoleInput)
	impact, err = s.Impact(ctx, "tenant", "admin", viewer.ID)
	require.NoError(t, err)
	err = s.DeleteRole(ctx, "tenant", "admin", viewer.ID, "", impact.Version, impact.Fingerprint)
	require.ErrorIs(t, err, ErrRoleInput)
	require.NoError(t, s.DeleteRole(ctx, "tenant", "admin", viewer.ID, roles["contributor"].ID, impact.Version, impact.Fingerprint))
	f.check(t, f.req("member", "folder", AccessUpload), true)
	listed, err := s.Roles(ctx, "tenant", "admin", true)
	require.NoError(t, err)
	require.Len(t, listed, 5)
}

func TestUserCreationSeparatesInternalIdentityFromAuthority(t *testing.T) {
	f, _, roles := roleFixture(t)
	ctx := context.Background()
	user := models.User{ID: "new-member", TenantID: "tenant", Email: "new-member@example.test", Role: "manager"}
	require.NoError(t, f.db.Create(&user).Error)
	input := UserAccessInput{UserType: "internal", OrganizationRole: "member", WorkspaceAccess: &[]WorkspaceRoleInput{}}
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return ApplyUserAccess(ctx, tx, "tenant", "admin", user, input) }))
	profile, err := AccessProfileFor(f.db, user)
	require.NoError(t, err)
	require.Equal(t, "internal", profile.UserType)
	require.False(t, profile.CanManageUsers || profile.AllWorkspaces)
	f.check(t, f.req(user.ID, "file", AccessRead), false)
	input.WorkspaceAccess = &[]WorkspaceRoleInput{{ClientID: "client", RoleID: roles["viewer"].ID}}
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return ApplyUserAccess(ctx, tx, "tenant", "admin", user, input) }))
	f.check(t, f.req(user.ID, "file", AccessRead), true)
	f.check(t, f.req(user.ID, "folder", AccessUpload), false)
	f.check(t, f.req(user.ID, "", AccessManageClient), false)
	require.ErrorIs(t, f.db.Transaction(func(tx *gorm.DB) error { return ApplyUserAccess(ctx, tx, "tenant", "member", user, input) }), ErrRoleForbidden)
	// Sharing management without deletion cannot delete the client workspace.
	custom := roles["workspace-manager"]
	custom.CanDelete = false
	custom.TemplateKey = nil
	custom.ID = "limited-manager"
	custom.Name = "Sharing coordinator"
	require.NoError(t, f.db.Create(&custom).Error)
	input.WorkspaceAccess = &[]WorkspaceRoleInput{{ClientID: "client", RoleID: custom.ID}}
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return ApplyUserAccess(ctx, tx, "tenant", "admin", user, input) }))
	f.check(t, f.req(user.ID, "", AccessManageClient), true)
	f.check(t, f.req(user.ID, "", AccessDeleteClient), false)
}

func TestAdditiveRoleInvitationsRemainIndependentAndRemovable(t *testing.T) {
	f, _, roles := roleFixture(t)
	ctx := context.Background()
	service := &SharingService{DB: f.db, TokenSecret: "secret", Now: func() time.Time { return accessTestNow }}
	grant, err := service.CreateGrant(ctx, f.req("admin", "folder", AccessShare), GrantInput{UserID: "recipient", RoleID: roles["viewer"].ID})
	require.NoError(t, err)
	invite, err := service.CreateInvitation(ctx, f.req("admin", "folder", AccessShare), InvitationInput{Email: "recipient@example.test", RoleID: roles["contributor"].ID}, "extra-role")
	require.NoError(t, err)
	_, err = service.AcceptInvitation(ctx, "tenant", "recipient", service.invitationToken(invite.ID))
	require.NoError(t, err)
	f.check(t, f.req("recipient", "folder", AccessUpload), true)
	view, err := service.ItemAccess(ctx, f.req("admin", "folder", AccessShare))
	require.NoError(t, err)
	require.Len(t, view.AdditionalAccess, 1)
	require.NoError(t, service.UpdateRoleAssignment(ctx, "tenant", "admin", view.AdditionalAccess[0].ID, view.AdditionalAccess[0].Version, "", nil, true))
	f.check(t, f.req("recipient", "folder", AccessUpload), false)
	f.check(t, f.req("recipient", "file", AccessRead), true)
	var current models.ItemGrant
	require.NoError(t, f.db.First(&current, "id = ?", grant.ID).Error)
	require.Equal(t, grant.RoleID, current.RoleID)
}

func TestIndependentSharingRightsCannotLeakThroughLegacyEndpoints(t *testing.T) {
	f, _, roles := roleFixture(t)
	ctx := context.Background()
	role := roles["viewer"]
	role.ID = "link-author"
	role.TemplateKey = nil
	role.Name = "Link author"
	role.CanCreateLinks = true
	require.NoError(t, f.db.Create(&role).Error)
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id='member'").Updates(map[string]any{"role_id": role.ID, "capabilities_set": true, "can_read": true, "can_share": true}).Error)
	service := &SharingService{DB: f.db, TokenSecret: "test", LinkSigningKey: "test", Now: func() time.Time { return accessTestNow }}
	link, err := service.CreateLink(ctx, f.req("member", "", AccessShare), LinkInput{AccessType: "read", ExpiryDays: 7}, "link-author")
	require.NoError(t, err)
	require.NotEmpty(t, link.ID)
	_, err = service.CreateInvitation(ctx, f.req("member", "", AccessShare), InvitationInput{Email: "other@example.test", RoleID: roles["viewer"].ID}, "not-inviter")
	require.Error(t, err)
	// The old link creation endpoint must also enforce CreateLinks, not CanShare.
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id='member'").Update("role_id", roles["client-contact"].ID).Error)
	err = service.PersistClientLinkForActor(ctx, f.req("member", "", AccessShare), &models.SecureLink{AccessType: "read"})
	require.Error(t, err)
	// Partial sharing bits cannot be persisted through a legacy four-bit grant:
	// that would discard the split rights and expand them into all sharing powers.
	caps := models.RolePermissions{CanRead: true, CanInviteExternal: true}.Capabilities()
	_, err = service.CreateGrant(ctx, f.req("admin", "folder", AccessShare), GrantInput{UserID: "recipient", Capabilities: caps})
	require.ErrorIs(t, err, ErrSharingInput)
	_, err = service.CreateInvitation(ctx, f.req("admin", "folder", AccessShare), InvitationInput{Email: "other@example.test", Capabilities: caps}, "bad-split")
	require.ErrorIs(t, err, ErrSharingInput)
	_, err = service.CreateInvitation(ctx, f.req("admin", "folder", AccessShare), InvitationInput{Email: "unknown@example.test", UserType: "internal", RoleID: roles["viewer"].ID}, "no-internal-identity")
	require.ErrorIs(t, err, ErrSharingInput)
}

func TestManageAccessDoesNotRequireExternalInvitationOrLinkPermission(t *testing.T) {
	f, _, roles := roleFixture(t)
	ctx := context.Background()
	role := roles["viewer"]
	role.ID = "access-coordinator"
	role.Name = "Access coordinator"
	role.TemplateKey = nil
	role.CanManageAccess = true
	require.NoError(t, f.db.Create(&role).Error)
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id='member'").Updates(map[string]any{"role_id": role.ID, "capabilities_set": true, "can_read": true, "can_share": true}).Error)
	viewer := roles["viewer"]
	require.NoError(t, f.db.Create(&models.UserClient{TenantID: "tenant", ClientID: "client", UserID: "recipient", RoleID: &viewer.ID, CapabilitiesSet: true, AccessCapabilities: viewer.RolePermissions.Capabilities()}).Error)
	s := &SharingService{DB: f.db, TokenSecret: "test", LinkSigningKey: "test"}
	req := f.req("member", "", AccessShare)
	require.NoError(t, s.ChangeMemberRole(ctx, req, "recipient", viewer.ID, viewer.ID))
	require.ErrorIs(t, s.ChangeMemberRole(ctx, req, "recipient", viewer.ID, "outdated-role"), ErrSharingConflict)
	_, err := s.CreateInvitation(ctx, req, InvitationInput{Email: "new-external@example.test", RoleID: viewer.ID}, "cannot-invite")
	require.Error(t, err)
	_, err = s.CreateLink(ctx, req, LinkInput{AccessType: "read", ExpiryDays: 7}, "cannot-link")
	require.Error(t, err)
	require.NoError(t, s.RemoveMember(ctx, req, "recipient", false))
	f.check(t, f.req("recipient", "file", AccessRead), false)
}

func TestRoleBackedGrantReplayKeepsRoleIdentity(t *testing.T) {
	f, _, roles := roleFixture(t)
	ctx := context.Background()
	s := &SharingService{DB: f.db}
	input := GrantInput{UserID: "recipient", RoleID: roles["client-contact"].ID}
	grant, err := s.CreateGrant(ctx, f.req("admin", "folder", AccessShare), input)
	require.NoError(t, err)
	replayed, err := s.CreateGrant(ctx, f.req("admin", "folder", AccessShare), input)
	require.NoError(t, err)
	require.Equal(t, grant.ID, replayed.ID)
	copy := roles["client-contact"]
	copy.ID = "renamable-independent-role"
	copy.TemplateKey = nil
	copy.Name = "Independent contact"
	require.NoError(t, f.db.Create(&copy).Error)
	input.RoleID = copy.ID
	_, err = s.CreateGrant(ctx, f.req("admin", "folder", AccessShare), input)
	require.ErrorIs(t, err, ErrSharingConflict, "identical current permissions do not make distinct editable roles interchangeable")
}

func TestWorkspaceRoleInvitationDoesNotOverwriteIndependentMembership(t *testing.T) {
	f, _, roles := roleFixture(t)
	ctx := context.Background()
	viewer := roles["viewer"]
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id='member'").Updates(map[string]any{"role_id": viewer.ID, "capabilities_set": true, "can_read": true, "can_upload": false, "can_share": false}).Error)
	s := &SharingService{DB: f.db, TokenSecret: "test", Now: func() time.Time { return accessTestNow }}
	invite, err := s.CreateInvitation(ctx, f.req("admin", "", AccessShare), InvitationInput{Email: "member@example.test", RoleID: roles["contributor"].ID}, "workspace-additive")
	require.NoError(t, err)
	_, err = s.AcceptInvitation(ctx, "tenant", "member", s.invitationToken(invite.ID))
	require.NoError(t, err)
	f.check(t, f.req("member", "folder", AccessUpload), true)
	var member models.UserClient
	require.NoError(t, f.db.First(&member, "user_id='member'").Error)
	require.Equal(t, viewer.ID, *member.RoleID)
	require.Equal(t, "independent", member.AccessSource)
	view, err := s.ClientAccess(ctx, f.req("admin", "", AccessShare))
	require.NoError(t, err)
	require.Len(t, view.AdditionalAccess, 1)
	require.NoError(t, s.UpdateRoleAssignment(ctx, "tenant", "admin", view.AdditionalAccess[0].ID, view.AdditionalAccess[0].Version, "", nil, true))
	f.check(t, f.req("member", "folder", AccessUpload), false)
	f.check(t, f.req("member", "file", AccessRead), true)
}

func TestGroupDetailsExposeExactAssignmentScopes(t *testing.T) {
	_, s, roles := roleFixture(t)
	input := GroupInput{Name: "Scoped team", MemberIDs: []string{}, Assignments: []GroupAssignmentInput{
		{ClientID: "client", RoleID: roles["viewer"].ID},
		{ClientID: "client", FileID: accessPtr("folder"), RoleID: roles["viewer"].ID},
		{ClientID: "client", FileID: accessPtr("file"), RoleID: roles["viewer"].ID},
	}}
	impact, err := s.PreviewGroup(t.Context(), "tenant", "admin", "", input)
	require.NoError(t, err)
	input.Impact = impact.Fingerprint
	saved, err := s.SaveGroup(t.Context(), "tenant", "admin", "", input)
	require.NoError(t, err)
	groups, err := s.Groups(t.Context(), "tenant", "admin")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	for _, group := range []*GroupDetails{saved, &groups[0]} {
		scopes := map[string]string{}
		for _, assignment := range group.Assignments {
			key := "workspace"
			if assignment.FileID != nil {
				key = *assignment.FileID
			}
			scopes[key] = assignment.ScopeType
		}
		require.Equal(t, map[string]string{"workspace": "workspace", "folder": "folder", "file": "file"}, scopes)
	}
}
