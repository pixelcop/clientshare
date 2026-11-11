package main

import (
	"context"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func TestPostgresRoleRevocationSerializesGrantCeilings(t *testing.T) {
	sqlDB, cfg := openPostgresMigrationTestDB(t)
	p := newMigrationProviderForTest(t, sqlDB, goose.DialectPostgres, cfg)
	_, err := p.Up(t.Context())
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	tenantID := tenant.DefaultTenantID
	admin := models.User{TenantID: tenantID, Role: "admin", Email: "role-race-admin@example.test"}
	actor := models.User{TenantID: tenantID, Role: "manager", Email: "role-race-actor@example.test"}
	recipient := models.User{TenantID: tenantID, Role: "client", Email: "role-race-recipient@example.test"}
	for _, user := range []*models.User{&admin, &actor, &recipient} {
		require.NoError(t, db.Create(user).Error)
	}
	client := models.Client{TenantID: tenantID, Name: "Role race", FolderPath: "role-race"}
	require.NoError(t, db.Create(&client).Error)
	folder := models.File{TenantID: tenantID, ClientID: client.ID, Type: "folder", Filename: "Root", Path: "role-race"}
	require.NoError(t, db.Create(&folder).Error)
	require.NoError(t, db.Model(&client).Update("root_folder_id", folder.ID).Error)
	var contact models.AccessRole
	require.NoError(t, db.Where("tenant_id=? AND template_key='client-contact'", tenantID).First(&contact).Error)
	require.NoError(t, db.Create(&models.UserClient{TenantID: tenantID, UserID: actor.ID, ClientID: client.ID, RoleID: &contact.ID, CapabilitiesSet: true, AccessCapabilities: contact.RolePermissions.Capabilities()}).Error)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tx := db.WithContext(ctx).Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, services.LockTenantAccess(tx, tenantID))
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		close(started)
		_, err := (&services.SharingService{DB: db}).CreateGrant(ctx, services.AccessRequest{TenantID: tenantID, ClientID: client.ID, FileID: folder.ID, Actor: services.AccessActor{UserID: actor.ID}}, services.GrantInput{UserID: recipient.ID, Capabilities: models.AccessCapabilities{CanRead: true}})
		result <- err
	}()
	<-started
	roleService := &services.AccessRoleService{DB: tx}
	impact, err := roleService.Impact(ctx, tenantID, admin.ID, contact.ID)
	require.NoError(t, err)
	contact.CanInviteExternal = false
	_, err = roleService.SaveRole(ctx, tenantID, admin.ID, contact.ID, contact, contact.Version, impact.Fingerprint)
	require.NoError(t, err)
	require.NoError(t, tx.Commit().Error)
	require.Error(t, <-result, "grant cannot commit using the ceiling from before revocation")
	var count int64
	require.NoError(t, db.Model(&models.ItemGrant{}).Where("tenant_id=? AND user_id=?", tenantID, recipient.ID).Count(&count).Error)
	require.Zero(t, count)
}
