package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	dbpkg "github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/migration"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestPendingInvitationContactsBackfillIsTenantLocalAndGrantsNoAccess(t *testing.T) {
	sqlDB := openMigrationTestDB(t)
	p := newMigrationProviderForTest(t, sqlDB, goose.DialectSQLite3, dbpkg.DatabaseConfig{Driver: dbpkg.DatabaseDriverSQLite})
	testPendingInvitationContactsBackfill(t, sqlDB, p)
}

func TestPendingInvitationContactsBackfillPostgres(t *testing.T) {
	sqlDB, cfg := openPostgresMigrationTestDB(t)
	p := newMigrationProviderForTest(t, sqlDB, goose.DialectPostgres, cfg)
	testPendingInvitationContactsBackfill(t, sqlDB, p)
}

func testPendingInvitationContactsBackfill(t *testing.T, sqlDB *sql.DB, p *goose.Provider) {
	t.Helper()
	ctx := context.Background()
	_, err := p.UpTo(ctx, 57)
	require.NoError(t, err)
	tx, err := sqlDB.Begin()
	require.NoError(t, err)
	db, err := migration.MigrationTxGormDB(tx)
	require.NoError(t, err)
	var tenant models.Tenant
	require.NoError(t, db.First(&tenant).Error)
	require.NoError(t, db.Create(&models.Tenant{ID: "foreign", Slug: "foreign", Name: "Foreign"}).Error)
	for _, client := range []models.Client{
		{ID: "one", TenantID: tenant.ID, Name: "One", FolderPath: "one"},
		{ID: "two", TenantID: tenant.ID, Name: "Two", FolderPath: "two"},
		{ID: "foreign", TenantID: "foreign", Name: "Foreign", FolderPath: "foreign"},
	} {
		require.NoError(t, db.Create(&client).Error)
	}
	for _, user := range []models.User{
		{ID: "existing", TenantID: tenant.ID, Email: "EXISTING@example.test", Role: "manager", PasswordHash: "keep-credential"},
		{ID: "foreign-user", TenantID: "foreign", Email: "new@example.test", Role: "admin"},
	} {
		require.NoError(t, db.Create(&user).Error)
	}
	require.NoError(t, db.Create(&models.AccessProfile{TenantID: tenant.ID, UserID: "existing", UserType: "internal", OrganizationRole: "member"}).Error)
	now := time.Now().UTC()
	for _, invite := range []models.AccessInvitation{
		{ID: "new", TenantID: tenant.ID, ClientID: "one", Email: "new@example.test", ExpiresAt: now.Add(time.Hour)},
		{ID: "same-email", TenantID: tenant.ID, ClientID: "two", Email: "NEW@example.test", ExpiresAt: now.Add(time.Hour)},
		{ID: "existing", TenantID: tenant.ID, ClientID: "one", Email: "existing@example.test", ExpiresAt: now.Add(time.Hour)},
		{ID: "foreign", TenantID: "foreign", ClientID: "foreign", Email: "new@example.test", ExpiresAt: now.Add(time.Hour)},
		{ID: "expired", TenantID: tenant.ID, ClientID: "one", Email: "expired@example.test", ExpiresAt: now.Add(-time.Hour)},
		{ID: "revoked", TenantID: tenant.ID, ClientID: "one", Email: "revoked@example.test", ExpiresAt: now.Add(time.Hour), RevokedAt: &now},
		{ID: "accepted", TenantID: tenant.ID, ClientID: "one", Email: "accepted@example.test", ExpiresAt: now.Add(time.Hour), AcceptedAt: &now},
	} {
		invite.TokenHash = "hash-" + invite.ID
		invite.CreatedBy = "existing"
		require.NoError(t, db.Create(&invite).Error)
	}
	require.NoError(t, tx.Commit())
	_, err = p.Up(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, scalarCount(t, sqlDB, `SELECT COUNT(*) FROM users WHERE email='new@example.test' AND tenant_id != 'foreign' AND role='client' AND password_hash=''`))
	require.EqualValues(t, 1, scalarCount(t, sqlDB, `SELECT COUNT(*) FROM access_profiles WHERE user_type='external' AND organization_role='none' AND NOT can_manage_users AND NOT all_workspaces`))
	require.EqualValues(t, 1, scalarCount(t, sqlDB, `SELECT COUNT(*) FROM access_invitations a JOIN access_invitations b ON a.recipient_user_id=b.recipient_user_id WHERE a.id='new' AND b.id='same-email'`))
	require.EqualValues(t, 1, scalarCount(t, sqlDB, `SELECT COUNT(*) FROM access_invitations WHERE id='existing' AND recipient_user_id='existing'`))
	require.EqualValues(t, 1, scalarCount(t, sqlDB, `SELECT COUNT(*) FROM access_invitations WHERE id='foreign' AND recipient_user_id='foreign-user'`))
	require.EqualValues(t, 1, scalarCount(t, sqlDB, `SELECT COUNT(*) FROM users WHERE id='existing' AND role='manager' AND password_hash='keep-credential'`))
	require.EqualValues(t, 0, scalarCount(t, sqlDB, `SELECT COUNT(*) FROM users WHERE email IN ('expired@example.test','revoked@example.test','accepted@example.test')`))
	for _, table := range []string{"user_clients", "item_grants", "invite_tokens", "passkey_credentials"} {
		require.EqualValues(t, 0, scalarCount(t, sqlDB, "SELECT COUNT(*) FROM "+table))
	}
}
