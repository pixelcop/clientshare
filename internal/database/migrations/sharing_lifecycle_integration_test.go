package main

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Use the migrated PostgreSQL schema and independent connections: a single
// SQLite connection cannot verify the row-lock protocol used in production.
func TestPostgresSharingAcceptanceAndRemovalRaces(t *testing.T) {
	sqlDB, cfg := openPostgresMigrationTestDB(t)
	provider := newMigrationProviderForTest(t, sqlDB, goose.DialectPostgres, cfg)
	_, err := provider.Up(t.Context())
	require.NoError(t, err)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(12)
	admin := models.User{TenantID: tenant.DefaultTenantID, Role: "admin", Email: "race-admin@example.test"}
	recipient := models.User{TenantID: tenant.DefaultTenantID, Role: "client", Email: "race-recipient@example.test"}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&recipient).Error)
	client := models.Client{TenantID: tenant.DefaultTenantID, Name: "Race fixture", FolderPath: "race-fixture"}
	require.NoError(t, db.Create(&client).Error)
	root := models.File{TenantID: tenant.DefaultTenantID, ClientID: client.ID, Type: "folder", Filename: "Root", Path: "race-fixture"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Model(&client).Update("root_folder_id", root.ID).Error)
	file := models.File{TenantID: tenant.DefaultTenantID, ClientID: client.ID, FolderID: &root.ID, Type: "file", Filename: "Shared", Path: "race-fixture/shared"}
	require.NoError(t, db.Create(&file).Error)
	service := &services.SharingService{DB: db, TokenSecret: "postgres-race-secret"}
	req := services.AccessRequest{TenantID: tenant.DefaultTenantID, ClientID: client.ID, FileID: file.ID, Actor: services.AccessActor{UserID: admin.ID}}
	input := services.InvitationInput{Email: recipient.Email, Capabilities: models.AccessCapabilities{CanRead: true}}
	invite, err := service.CreateInvitation(t.Context(), req, input, "simultaneous-acceptance")
	require.NoError(t, err)
	_, token, err := service.InvitationForDelivery(t.Context(), req, invite.ID)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.AcceptInvitation(t.Context(), req.TenantID, recipient.ID, token)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&models.ItemGrant{}).Where("tenant_id = ? AND user_id = ?", req.TenantID, recipient.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&models.UserClient{}).Where("tenant_id = ? AND user_id = ?", req.TenantID, recipient.ID).Count(&count).Error)
	require.Zero(t, count)
	for i := 0; i < 8; i++ {
		require.NoError(t, db.Create(&models.UserClient{TenantID: req.TenantID, ClientID: client.ID, UserID: recipient.ID}).Error)
		invite, err := service.CreateInvitation(t.Context(), req, input, fmt.Sprintf("accept-removal-%d", i))
		require.NoError(t, err)
		_, token, err := service.InvitationForDelivery(t.Context(), req, invite.ID)
		require.NoError(t, err)
		start := make(chan struct{})
		accept := make(chan error, 1)
		remove := make(chan error, 1)
		go func() {
			<-start
			_, err := service.AcceptInvitation(t.Context(), req.TenantID, recipient.ID, token)
			accept <- err
		}()
		go func() {
			<-start
			scope := req
			scope.FileID = ""
			remove <- service.RemoveMember(t.Context(), scope, recipient.ID, false)
		}()
		close(start)
		require.NoError(t, <-remove)
		if err := <-accept; err != nil {
			require.True(t, errors.Is(err, gorm.ErrRecordNotFound), "%v", err)
		}
		_, err = service.AcceptInvitation(t.Context(), req.TenantID, recipient.ID, token)
		if err != nil {
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
		}
		check := req
		check.Actor.UserID, check.Operation = recipient.ID, services.AccessRead
		d, err := (&services.AccessService{DB: db}).Resolve(t.Context(), check)
		require.NoError(t, err)
		require.False(t, d.Allowed, "duplicate acceptance cannot restore access after removal")
	}
}
