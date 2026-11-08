package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var accessTestNow = time.Date(2026, 11, 8, 17, 0, 0, 0, time.UTC)

func accessPtr[T any](v T) *T { return &v }

type accessFixture struct {
	db *gorm.DB
	s  *AccessService
}

func newAccessFixture(t *testing.T) accessFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Client{}, &models.File{}, &models.UserClient{}, &models.SecureLink{}, &models.ItemGrant{}))
	for _, row := range []any{
		&models.Client{ID: "client", TenantID: "tenant", Name: "Client", FolderPath: "client", RootFolderID: accessPtr("root")},
		&models.Client{ID: "other-client", TenantID: "tenant", Name: "Other", FolderPath: "other"},
		&models.Client{ID: "foreign-client", TenantID: "foreign", Name: "Foreign", FolderPath: "foreign"},
		&models.File{ID: "root", TenantID: "tenant", ClientID: "client", Type: "folder"},
		&models.File{ID: "folder", TenantID: "tenant", ClientID: "client", Type: "folder", FolderID: accessPtr("root")},
		&models.File{ID: "nested", TenantID: "tenant", ClientID: "client", Type: "folder", FolderID: accessPtr("folder")},
		&models.File{ID: "file", TenantID: "tenant", ClientID: "client", Type: "file", FolderID: accessPtr("nested")},
		&models.File{ID: "sibling", TenantID: "tenant", ClientID: "client", Type: "file", FolderID: accessPtr("root"), Path: "same/path/prefix"},
		&models.File{ID: "other-file", TenantID: "tenant", ClientID: "other-client", Type: "file"},
		&models.File{ID: "foreign-file", TenantID: "foreign", ClientID: "foreign-client", Type: "file"},
	} {
		require.NoError(t, db.Create(row).Error)
	}
	for _, user := range []models.User{
		{ID: "admin", Role: "admin"},
		{ID: "full", Role: "manager", ManagerFullAccess: true},
		{ID: "manager", Role: "manager"},
		{ID: "member", Role: "client"},
		{ID: "customer", Role: "customer"},
		{ID: "recipient", Role: "client"},
		{ID: "unknown-role", Role: "owner", ManagerFullAccess: true},
	} {
		user.TenantID, user.Email = "tenant", user.ID+"@example.test"
		require.NoError(t, db.Create(&user).Error)
	}
	for _, id := range []string{"manager", "member", "customer"} {
		require.NoError(t, db.Create(&models.UserClient{TenantID: "tenant", ClientID: "client", UserID: id}).Error)
	}
	return accessFixture{db: db, s: &AccessService{DB: db}}
}

func (f accessFixture) req(user, file string, op AccessOperation) AccessRequest {
	return AccessRequest{TenantID: "tenant", ClientID: "client", FileID: file, Actor: AccessActor{UserID: user}, Operation: op, Now: accessTestNow}
}

func (f accessFixture) check(t *testing.T, req AccessRequest, allowed bool) AccessDecision {
	t.Helper()
	d, err := f.s.Resolve(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, allowed, d.Allowed, "request: %+v; decision: %+v", req, d)
	return d
}

func (f accessFixture) grant(t *testing.T, file, user string, caps models.AccessCapabilities) *models.ItemGrant {
	t.Helper()
	g := &models.ItemGrant{TenantID: "tenant", ClientID: "client", FileID: file, UserID: user, AccessCapabilities: caps, CreatedBy: "removed-creator"}
	require.NoError(t, f.db.Create(g).Error)
	return g
}

func TestAccessRoleMembershipAndOperationMatrix(t *testing.T) {
	f := newAccessFixture(t)
	for _, user := range []string{"admin", "full", "manager", "member", "customer", "recipient", "unknown-role", "missing"} {
		t.Run(user, func(t *testing.T) {
			hasClient := user == "admin" || user == "full" || user == "manager" || user == "member" || user == "customer"
			staff := user == "admin" || user == "full" || user == "manager"
			for _, op := range []AccessOperation{AccessRead, AccessDownload, AccessContext, AccessShare} {
				f.check(t, f.req(user, "file", op), hasClient)
			}
			for _, op := range []AccessOperation{AccessList, AccessZIP, AccessUpload} {
				f.check(t, f.req(user, "folder", op), hasClient)
				f.check(t, f.req(user, "file", op), false)
			}
			f.check(t, f.req(user, "file", AccessDelete), staff)
			f.check(t, f.req(user, "", AccessInviteClient), hasClient)
			f.check(t, f.req(user, "", AccessManageClient), staff)
			f.check(t, f.req(user, "", AccessManageQueue), staff)
			f.check(t, f.req(user, "file", AccessInviteClient), false)
			f.check(t, f.req(user, "file", "unknown"), false)
			r := f.req(user, "other-file", AccessRead)
			r.ClientID = "other-client"
			f.check(t, r, user == "admin" || user == "full")
		})
	}
	// Full access covers clients created after migration too.
	require.NoError(t, f.db.Create(&models.Client{ID: "future", TenantID: "tenant", FolderPath: "future"}).Error)
	r := f.req("full", "", AccessRead)
	r.ClientID = "future"
	f.check(t, r, true)
	r.Actor.UserID = "manager"
	f.check(t, r, false)
}

func TestAccessExplicitMembershipAndAdditiveItemGrants(t *testing.T) {
	f := newAccessFixture(t)
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id = ?", "member").Updates(map[string]any{"capabilities_set": true, "can_read": true}).Error)
	f.check(t, f.req("member", "folder", AccessRead), true)
	f.check(t, f.req("member", "folder", AccessUpload), false)
	f.check(t, f.req("member", "folder", AccessShare), false)
	f.grant(t, "folder", "member", models.AccessCapabilities{CanUpload: true, CanShare: true})
	f.check(t, f.req("member", "folder", AccessUpload), true)
	f.check(t, f.req("member", "file", AccessRead), true)
	f.check(t, f.req("member", "file", AccessUpload), false)
	d := f.check(t, f.req("member", "folder", AccessShare), true)
	require.Len(t, d.Sources, 2)
	require.True(t, d.CanGrant(models.AccessCapabilities{CanRead: true, CanUpload: true, CanShare: true}))
	require.False(t, d.CanGrant(models.AccessCapabilities{CanDelete: true}))
	f.grant(t, "file", "member", models.AccessCapabilities{}) // cannot reduce inheritance
	f.check(t, f.req("member", "file", AccessRead), true)
	require.NoError(t, f.db.Model(&models.UserClient{}).Where("user_id = ?", "manager").Update("capabilities_set", true).Error)
	f.check(t, f.req("manager", "folder", AccessDelete), false) // explicit zero is not legacy
}

func TestAccessFileAndFolderGrantBoundaries(t *testing.T) {
	for _, root := range []string{"file", "folder", "root"} {
		t.Run(root, func(t *testing.T) {
			f := newAccessFixture(t)
			f.grant(t, root, "recipient", models.AccessCapabilities{CanRead: true, CanUpload: true, CanShare: true})
			f.check(t, f.req("recipient", "file", AccessRead), true)
			f.check(t, f.req("recipient", "file", AccessShare), true)
			f.check(t, f.req("recipient", "file", AccessUpload), false)
			f.check(t, f.req("recipient", "file", AccessDelete), false)
			f.check(t, f.req("recipient", "folder", AccessList), root != "file")
			f.check(t, f.req("recipient", "folder", AccessUpload), root != "file")
			f.check(t, f.req("recipient", "root", AccessZIP), root == "root")
			f.check(t, f.req("recipient", "sibling", AccessRead), root == "root")
			for _, op := range []AccessOperation{AccessRead, AccessShare, AccessInviteClient, AccessManageClient} {
				f.check(t, f.req("recipient", "", op), false) // even root grants are not membership
			}
			// Future descendants inherit; string paths do not grant siblings.
			require.NoError(t, f.db.Create(&models.File{ID: "future", TenantID: "tenant", ClientID: "client", Type: "file", FolderID: accessPtr("folder")}).Error)
			f.check(t, f.req("recipient", "future", AccessRead), root != "file")
		})
	}
}

func TestAccessExpiryRevocationAndOnwardIndependence(t *testing.T) {
	f := newAccessFixture(t)
	g := f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true})
	r := f.req("recipient", "file", AccessRead)
	f.check(t, r, true) // creator no longer exists
	// Store a future instant in EST whose wall-clock appears earlier than UTC.
	expiry := accessTestNow.Add(time.Hour).In(time.FixedZone("EST", -5*3600))
	require.NoError(t, f.db.Model(g).Update("expires_at", expiry).Error)
	f.check(t, r, true)
	r.Now = expiry
	f.check(t, r, false) // exact expiry is invalid, including already-open sessions
	r.Now = accessTestNow
	require.NoError(t, f.db.Model(g).Update("revoked_at", accessTestNow).Error)
	f.check(t, r, false)
	require.NoError(t, f.db.Model(g).Updates(map[string]any{"revoked_at": nil, "expires_at": nil}).Error)
	f.check(t, r, true)
	require.NoError(t, f.db.Delete(&models.User{}, "id = ?", "recipient").Error)
	f.check(t, r, false) // current tenant-local identity must still exist
}

func TestAccessLinkOnlyScopeAndModes(t *testing.T) {
	for _, scope := range []string{"", "folder", "file"} {
		for _, mode := range []string{"read", "view", "write", "readwrite", "invalid"} {
			t.Run(scope+"/"+mode, func(t *testing.T) {
				f := newAccessFixture(t)
				link := &models.SecureLink{ID: "link", TenantID: "tenant", ClientID: "client", Token: "secret", AccessType: mode, ExpiresAt: accessTestNow.Add(time.Hour).In(time.FixedZone("EST", -5*3600)), CreatedBy: "removed"}
				if scope != "" {
					link.FileID = &scope
				}
				require.NoError(t, f.db.Create(link).Error)
				r := f.req("admin", "file", AccessRead) // staff must not leak into link view
				r.Actor.LinkID = link.ID
				valid := mode != "invalid" && (scope != "file" || mode == "read" || mode == "view")
				read := mode == "read" || mode == "view" || mode == "readwrite"
				upload := mode == "write" || mode == "readwrite"
				f.check(t, r, valid && read)
				r.Operation = AccessDelete
				f.check(t, r, false)
				r.Operation = AccessShare
				d := f.check(t, r, false)
				require.False(t, d.CanGrant(models.AccessCapabilities{CanRead: true}))
				r.FileID, r.Operation = "folder", AccessUpload
				knownFolder := scope == "folder" || read
				f.check(t, r, valid && scope != "file" && upload && knownFolder)
				r.Operation = AccessList
				f.check(t, r, valid && scope != "file" && read)
				r.Operation = AccessContext
				f.check(t, r, valid && scope != "file" && knownFolder)
				r.FileID, r.Operation = "sibling", AccessRead
				f.check(t, r, valid && scope == "" && read)
				r.FileID = "root"
				f.check(t, r, valid && scope == "" && read)
				r.FileID = ""
				f.check(t, r, valid && scope == "" && read)
				r.FileID, r.Now = "file", link.ExpiresAt
				f.check(t, r, false)
				r.Now = accessTestNow
				require.NoError(t, f.db.Model(link).Update("revoked_at", accessTestNow).Error)
				f.check(t, r, false)
			})
		}
	}
}

func TestAccessTenantAndCorruptAncestryFailClosed(t *testing.T) {
	for _, corruption := range []string{"deleted-target", "deleted-ancestor", "disk-deleted", "missing-parent", "foreign-parent", "other-client-parent", "file-parent", "cycle", "self-cycle", "unknown-type", "depth"} {
		t.Run(corruption, func(t *testing.T) {
			f := newAccessFixture(t)
			f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true})
			switch corruption {
			case "deleted-target":
				require.NoError(t, f.db.Model(&models.File{}).Where("id = 'file'").Update("deleted_at", accessTestNow).Error)
			case "deleted-ancestor":
				require.NoError(t, f.db.Model(&models.File{}).Where("id = 'folder'").Update("deleted_at", accessTestNow).Error)
			case "disk-deleted":
				require.NoError(t, f.db.Model(&models.File{}).Where("id = 'file'").Update("disk_deleted_at", accessTestNow).Error)
			case "missing-parent", "foreign-parent", "other-client-parent", "file-parent", "self-cycle":
				parent := map[string]string{"missing-parent": "missing", "foreign-parent": "foreign-file", "other-client-parent": "other-file", "file-parent": "sibling", "self-cycle": "file"}[corruption]
				require.NoError(t, f.db.Model(&models.File{}).Where("id = 'file'").Update("folder_id", parent).Error)
			case "cycle":
				require.NoError(t, f.db.Model(&models.File{}).Where("id = 'root'").Update("folder_id", "nested").Error)
			case "unknown-type":
				require.NoError(t, f.db.Model(&models.File{}).Where("id = 'folder'").Update("type", "symlink").Error)
			case "depth":
				parent := "nested"
				for i := 0; i < maxAccessDepth; i++ {
					id := fmt.Sprintf("deep-%d", i)
					require.NoError(t, f.db.Create(&models.File{ID: id, TenantID: "tenant", ClientID: "client", Type: "folder", FolderID: &parent}).Error)
					parent = id
				}
				require.NoError(t, f.db.Model(&models.File{}).Where("id = 'file'").Update("folder_id", parent).Error)
			}
			for _, actor := range []AccessActor{{UserID: "admin"}, {UserID: "recipient"}, {LinkID: "link"}} {
				if actor.LinkID != "" {
					require.NoError(t, f.db.Create(&models.SecureLink{ID: "link", TenantID: "tenant", ClientID: "client", Token: "link", AccessType: "read", FileID: accessPtr("file"), ExpiresAt: accessTestNow.Add(time.Hour)}).Error)
				}
				r := f.req("", "file", AccessRead)
				r.Actor = actor
				f.check(t, r, false)
			}
		})
	}
	f := newAccessFixture(t)
	for _, change := range []func(*AccessRequest){
		func(r *AccessRequest) { r.TenantID = "" },
		func(r *AccessRequest) { r.TenantID = "foreign" },
		func(r *AccessRequest) { r.ClientID = "missing" },
		func(r *AccessRequest) { r.ClientID = "other-client" },
		func(r *AccessRequest) { r.FileID = "foreign-file" },
		func(r *AccessRequest) { r.Actor = AccessActor{} },
		func(r *AccessRequest) { r.Actor.LinkID = "missing" },
	} {
		r := f.req("admin", "file", AccessRead)
		change(&r)
		f.check(t, r, false)
	}
	// Corrupt grants from other scopes must not be salvaged by a matching ID.
	for _, g := range []models.ItemGrant{
		{TenantID: "foreign", ClientID: "client", UserID: "recipient", FileID: "file"},
		{TenantID: "tenant", ClientID: "other-client", UserID: "recipient", FileID: "file"},
	} {
		g.CanRead = true
		require.NoError(t, f.db.Create(&g).Error)
	}
	f.check(t, f.req("recipient", "file", AccessRead), false)
	require.NoError(t, f.db.Delete(&models.Client{}, "id = 'client'").Error)
	f.check(t, f.req("admin", "file", AccessRead), false)
}

func TestAccessReparentingAndBatchQueries(t *testing.T) {
	f := newAccessFixture(t)
	f.grant(t, "folder", "recipient", models.AccessCapabilities{CanRead: true})
	f.check(t, f.req("recipient", "file", AccessRead), true)
	require.NoError(t, f.db.Model(&models.File{}).Where("id = 'file'").Update("folder_id", "root").Error)
	f.check(t, f.req("recipient", "file", AccessRead), false)
	f.grant(t, "file", "recipient", models.AccessCapabilities{CanRead: true})
	f.check(t, f.req("recipient", "file", AccessRead), true) // direct survives same-client move
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = fmt.Sprintf("batch-%d", i)
		require.NoError(t, f.db.Create(&models.File{ID: ids[i], TenantID: "tenant", ClientID: "client", Type: "file", FolderID: accessPtr("folder")}).Error)
	}
	queries := 0
	require.NoError(t, f.db.Callback().Query().Before("gorm:query").Register("count-access", func(*gorm.DB) { queries++ }))
	defer f.db.Callback().Query().Remove("count-access")
	results, err := f.s.ResolveItems(context.Background(), f.req("recipient", "", AccessRead), ids)
	require.NoError(t, err)
	for _, id := range ids {
		require.True(t, results[id].Allowed)
	}
	// client/user/membership + 3 ancestry levels + one grant batch, not N+1.
	require.LessOrEqual(t, queries, 7)
	_, err = f.s.ResolveItems(context.Background(), f.req("recipient", "", AccessRead), make([]string, maxAccessItems+1))
	require.Error(t, err)
}

func TestClientScopeMatchesResolverWithoutTrustingRoleStrings(t *testing.T) {
	f := newAccessFixture(t)
	f.grant(t, "root", "recipient", models.AccessCapabilities{CanRead: true, CanShare: true})
	for _, user := range []string{"admin", "full", "manager", "member", "customer", "recipient", "missing", "unknown-role"} {
		for _, op := range []AccessOperation{AccessContext, AccessRead, AccessShare, AccessInviteClient, AccessManageClient, AccessManageQueue} {
			q, err := f.s.ClientScope(t.Context(), "tenant", user, op)
			require.NoError(t, err)
			var clients []models.Client
			require.NoError(t, q.Find(&clients).Error)
			ids := map[string]bool{}
			for _, client := range clients {
				ids[client.ID] = true
			}
			for _, id := range []string{"client", "other-client"} {
				r := f.req(user, "", op)
				r.ClientID = id
				d, err := f.s.Resolve(t.Context(), r)
				require.NoError(t, err)
				require.Equal(t, d.Allowed, ids[id], "user=%s op=%s client=%s", user, op, id)
			}
		}
	}
	service := &ClientService{DB: f.db}
	clients, err := service.ListClients(t.Context(), "tenant", "recipient", "admin")
	require.NoError(t, err)
	require.Empty(t, clients)
}
