package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestViewNamePortableMapping(t *testing.T) {
	for _, tc := range []struct{ logical, projected string }{
		{"Example Consulting P.C.", "Example Consulting P.C_"},
		{"report. ", "report__"}, {"...", "___"}, {"   ", "___"},
		{"a:*?\"<>|b", "a_______b"}, {"a\t\n\x7f\u0085\u200db", "a_____b"},
		{"CON", "_CON"}, {"con.txt", "_con.txt"}, {"NUL .txt", "_NUL .txt"},
		{"PRN", "_PRN"}, {"AUX", "_AUX"}, {"COM1.txt", "_COM1.txt"},
		{"LPT9", "_LPT9"}, {"COM¹.txt", "_COM¹.txt"}, {"CONOUT$", "_CONOUT$"},
		{"COM0", "COM0"}, {"LPT10", "LPT10"}, {"company.txt", "company.txt"},
		{"Résumé 2026.pdf", "Résumé 2026.pdf"}, {"under_score", "under_score"},
	} {
		t.Run(tc.logical, func(t *testing.T) {
			actual, err := viewName(tc.logical)
			require.NoError(t, err)
			require.Equal(t, tc.projected, actual)
			// Already projected names never acquire additional escaping.
			again, err := viewName(actual)
			require.NoError(t, err)
			require.Equal(t, actual, again)
		})
	}
	for _, name := range []string{"", ".", "..", "../escape", "a/b", `a\b`, "a\x00b", string([]byte{0xff}), strings.Repeat("x", 256)} {
		_, err := viewName(name)
		require.Error(t, err, "%q", name)
	}
}

func TestReadableViewTrailingNamesHardlinkRehearsal(t *testing.T) {
	db, root, client, file := migrationFixture(t)
	ctx := context.Background()
	// Rehearse an existing trailing-period client/root before inventory.
	name := "Example Consulting P.C."
	require.NoError(t, db.Model(client).Updates(map[string]interface{}{"name": name, "folder_path": name}).Error)
	require.NoError(t, db.Model(&models.File{}).Where("id = ?", *client.RootFolderID).Update("filename", name).Error)
	require.NoError(t, RefreshLogicalPaths(db, "tenant", client.ID))
	require.NoError(t, os.Rename(filepath.Join(root, "tenants/tenant/Acme"), filepath.Join(root, "tenants/tenant", name)))
	plan, err := InventoryObjects(ctx, db, root, nil)
	require.NoError(t, err)
	require.Empty(t, plan.Problems)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	require.NoError(t, db.First(file, "id = ?", file.ID).Error)
	checksum, key := file.Checksum, *file.StorageKey
	folder := &models.File{TenantID: "tenant", ClientID: client.ID, FolderID: client.RootFolderID, Filename: "Nested. ", Type: "folder"}
	require.NoError(t, db.Create(folder).Error)
	require.NoError(t, NewFileService(db).RenameMove(ctx, "tenant", file.ID, "report.txt. ", folder.ID))
	view, err := NewReadableViews(db, root, "", ViewModeHardlink)
	require.NoError(t, err)
	require.NoError(t, view.Probe())
	lock, err := view.Lock()
	require.NoError(t, err)
	defer lock.Close()
	var state models.ReadableView
	reload := func() {
		require.NoError(t, db.First(&state, "tenant_id = ? AND client_id = ?", "tenant", client.ID).Error)
	}
	reload()
	before := state
	var output bytes.Buffer
	require.NoError(t, view.Reconcile(ctx, state, true, &output))
	require.Contains(t, output.String(), `"Example Consulting P.C./Nested. /report.txt. " -> "tenant/Example Consulting P.C_/Nested__/report.txt__"`)
	require.NoDirExists(t, filepath.Join(view.Root, "tenant"))
	reload()
	require.Equal(t, before, state)
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	old := filepath.Join(view.Root, "tenant/Example Consulting P.C_/Nested__/report.txt__")
	source := immutableObjectPathForTest(t, root, "tenant", key)
	verify := func(path string) {
		t.Helper()
		a, err := os.Stat(source)
		require.NoError(t, err)
		b, err := os.Stat(path)
		require.NoError(t, err)
		require.True(t, os.SameFile(a, b), "projection must share authoritative inode")
		actual, err := hashRegularFile(ctx, source)
		require.NoError(t, err)
		require.Equal(t, checksum, actual.Checksum)
		require.NoError(t, db.First(file, "id = ?", file.ID).Error)
		require.Equal(t, checksum, file.Checksum)
		require.Equal(t, key, *file.StorageKey)
	}
	verify(old)
	// No logical metadata was sanitized, including the root and nested names.
	var logicalRoot models.File
	require.NoError(t, db.First(&logicalRoot, "id = ?", *client.RootFolderID).Error)
	require.Equal(t, name, logicalRoot.Filename)
	require.NoError(t, db.First(client, "id = ?", client.ID).Error)
	require.Equal(t, name, client.Name)
	require.Equal(t, "report.txt. ", file.Filename)
	// Repeated reconciliation and process restart keep exactly the same mapping.
	reload()
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	view, err = NewReadableViews(db, root, "", ViewModeHardlink)
	require.NoError(t, err)
	reload()
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	verify(old)
	_, err = (&ClientService{DB: db}).UpdateClient(ctx, "tenant", client.ID, "Renamed.")
	require.NoError(t, err)
	require.NoError(t, NewFileService(db).RenameMove(ctx, "tenant", folder.ID, "Folder.", *client.RootFolderID))
	require.NoError(t, NewFileService(db).RenameMove(ctx, "tenant", file.ID, "CON.txt", folder.ID))
	reload()
	require.NoError(t, view.Reconcile(ctx, state, true, io.Discard))
	verify(old)
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	require.NoFileExists(t, old)
	verify(filepath.Join(view.Root, "tenant/Renamed_/Folder_/_CON.txt"))
}

func TestReadableViewNamingCollisionsDoNotApply(t *testing.T) {
	for i, pair := range [][2]string{{"report.", "report_"}, {"report ", "report_"}, {"a?b", "a*b"}, {"CON", "_CON"}, {"Report", "report"}, {"Résumé", "Re\u0301sume\u0301"}, {"Straße", "STRASSE"}} {
		t.Run(fmt.Sprintf("pair-%d", i), func(t *testing.T) {
			db, root, client, _ := migrationFixture(t)
			plan, err := InventoryObjects(context.Background(), db, root, nil)
			require.NoError(t, err)
			require.NoError(t, ApplyObjects(context.Background(), db, root, plan))
			for _, name := range pair {
				require.NoError(t, db.Create(&models.File{TenantID: "tenant", ClientID: client.ID, FolderID: client.RootFolderID, Filename: name, Type: "folder"}).Error)
			}
			view, err := NewReadableViews(db, root, "", ViewModeHardlink)
			require.NoError(t, err)
			var state models.ReadableView
			require.NoError(t, db.First(&state, "client_id = ?", client.ID).Error)
			for _, dry := range []bool{true, false} {
				err := view.Reconcile(context.Background(), state, dry, io.Discard)
				require.ErrorContains(t, err, "projection collision")
				require.Contains(t, err.Error(), pair[0])
				require.Contains(t, err.Error(), pair[1])
				require.NoDirExists(t, view.Root)
				var after models.ReadableView
				require.NoError(t, db.First(&after, "client_id = ?", client.ID).Error)
				require.Equal(t, state.Manifest, after.Manifest)
			}
		})
	}
}

func TestReadableViewClientRootCollisionBeforeEitherApplied(t *testing.T) {
	db, root, client, _ := migrationFixture(t)
	other, err := (&ClientService{DB: db}).CreateClient(context.Background(), "tenant", "Acme.")
	require.NoError(t, err)
	_, err = (&ClientService{DB: db}).UpdateClient(context.Background(), "tenant", client.ID, "Acme_")
	require.NoError(t, err)
	view, err := NewReadableViews(db, root, "", ViewModeHardlink)
	require.NoError(t, err)
	for _, id := range []string{client.ID, other.ID} {
		var state models.ReadableView
		require.NoError(t, db.First(&state, "client_id = ?", id).Error)
		require.ErrorContains(t, view.Reconcile(context.Background(), state, false, io.Discard), "projection root collision")
		require.NoDirExists(t, view.Root)
	}
}

func TestReadableViewMappedUnmanagedCollisionPreservesManifest(t *testing.T) {
	db, root, client, file := migrationFixture(t)
	ctx := context.Background()
	plan, err := InventoryObjects(ctx, db, root, nil)
	require.NoError(t, err)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	view, err := NewReadableViews(db, root, "", ViewModeHardlink)
	require.NoError(t, err)
	var state models.ReadableView
	require.NoError(t, db.First(&state, "client_id = ?", client.ID).Error)
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	require.NoError(t, db.First(&state, "client_id = ?", client.ID).Error)
	manifest := state.Manifest
	require.NoError(t, NewFileService(db).RenameMove(ctx, "tenant", file.ID, "new.", *client.RootFolderID))
	unmanaged := filepath.Join(view.Root, "tenant/Acme/new_")
	require.NoError(t, os.WriteFile(unmanaged, []byte("do not touch"), 0600))
	for _, dry := range []bool{true, false} {
		require.ErrorContains(t, view.Reconcile(ctx, state, dry, io.Discard), "unmanaged view collision")
		body, err := os.ReadFile(unmanaged)
		require.NoError(t, err)
		require.Equal(t, "do not touch", string(body))
		require.FileExists(t, filepath.Join(view.Root, "tenant/Acme/report.txt"))
		var after models.ReadableView
		require.NoError(t, db.First(&after, "client_id = ?", client.ID).Error)
		require.Equal(t, manifest, after.Manifest)
		var parsed ViewManifest
		require.NoError(t, json.Unmarshal([]byte(after.Manifest), &parsed))
		require.Len(t, parsed.Entries, 2)
	}
}

func TestReadableViewUnsafeLogicalNamesNeverEscape(t *testing.T) {
	for i, name := range []string{".", "..", "../escape", "/absolute", `a\b`, "a\x00b"} {
		t.Run(fmt.Sprintf("unsafe-%d", i), func(t *testing.T) {
			db, root, client, file := migrationFixture(t)
			// Deliberately corrupt a record, bypassing the normal write validators.
			require.NoError(t, db.Model(file).Update("filename", name).Error)
			view, err := NewReadableViews(db, root, "", ViewModeHardlink)
			require.NoError(t, err)
			var state models.ReadableView
			require.NoError(t, db.First(&state, "client_id = ?", client.ID).Error)
			for _, dry := range []bool{true, false} {
				err := view.Reconcile(context.Background(), state, dry, io.Discard)
				require.ErrorContains(t, err, "unsafe projection name")
				require.Contains(t, err.Error(), file.ID)
				require.NoDirExists(t, view.Root)
			}
		})
	}
}

func TestReadableViewNormalizationOnlyRename(t *testing.T) {
	db, root, client, file := migrationFixture(t)
	ctx := context.Background()
	plan, err := InventoryObjects(ctx, db, root, nil)
	require.NoError(t, err)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	view, err := NewReadableViews(db, root, "", ViewModeHardlink)
	require.NoError(t, err)
	for _, name := range []string{"Résumé.", "Re\u0301sume\u0301.", "RÉSUMÉ."} {
		_, err := (&ClientService{DB: db}).UpdateClient(ctx, "tenant", client.ID, name)
		require.NoError(t, err)
		var state models.ReadableView
		require.NoError(t, db.First(&state, "client_id = ?", client.ID).Error)
		require.NoError(t, view.Reconcile(ctx, state, true, io.Discard))
		require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
		projected, err := viewName(name)
		require.NoError(t, err)
		a, err := os.Stat(filepath.Join(view.Root, "tenant", projected, "report.txt"))
		require.NoError(t, err)
		b, err := os.Stat(immutableObjectPathForTest(t, root, "tenant", file.ID))
		require.NoError(t, err)
		require.True(t, os.SameFile(a, b))
		entries, err := os.ReadDir(filepath.Join(view.Root, "tenant"))
		require.NoError(t, err)
		require.Len(t, entries, 1)
	}
}

func TestReadableViewUpgradeKeepsOwnedManifest(t *testing.T) {
	db, root, client, file := migrationFixture(t)
	ctx := context.Background()
	plan, err := InventoryObjects(ctx, db, root, nil)
	require.NoError(t, err)
	require.NoError(t, ApplyObjects(ctx, db, root, plan))
	require.NoError(t, db.First(file, "id = ?", file.ID).Error)
	_, err = (&ClientService{DB: db}).UpdateClient(ctx, "tenant", client.ID, "Legacy\u200d")
	require.NoError(t, err)
	view, err := NewReadableViews(db, root, "", ViewModeHardlink)
	require.NoError(t, err)
	var state models.ReadableView
	require.NoError(t, db.First(&state, "client_id = ?", client.ID).Error)
	// Simulate an older version's owned projection. That version accepted this
	// format character unchanged; upgrading must use, not discard, ownership.
	require.NoError(t, os.MkdirAll(filepath.Join(view.Root, "tenant"), 0700))
	old := ViewManifest{Mode: view.Mode, Root: view.Root, Entries: []ViewEntry{
		{Path: "tenant/Legacy\u200d", Directory: true},
		{Path: "tenant/Legacy\u200d/report.txt", Key: *file.StorageKey, Checksum: file.Checksum, Size: file.Size},
	}}
	for _, entry := range old.Entries {
		require.NoError(t, view.create(ctx, "tenant", entry))
	}
	require.NoError(t, view.save(state, old))
	require.NoError(t, db.First(&state, "client_id = ?", client.ID).Error)
	before, err := os.Stat(immutableObjectPathForTest(t, root, "tenant", file.ID))
	require.NoError(t, err)
	require.NoError(t, view.Reconcile(ctx, state, true, io.Discard))
	require.FileExists(t, filepath.Join(view.Root, "tenant/Legacy\u200d/report.txt"))
	require.NoDirExists(t, filepath.Join(view.Root, "tenant/Legacy_"))
	require.NoError(t, view.Reconcile(ctx, state, false, io.Discard))
	require.NoDirExists(t, filepath.Join(view.Root, "tenant/Legacy\u200d"))
	after, err := os.Stat(filepath.Join(view.Root, "tenant/Legacy_/report.txt"))
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
	require.Equal(t, before.Mode(), after.Mode())
	actual, err := hashRegularFile(ctx, immutableObjectPathForTest(t, root, "tenant", file.ID))
	require.NoError(t, err)
	require.Equal(t, file.Checksum, actual.Checksum)
}
