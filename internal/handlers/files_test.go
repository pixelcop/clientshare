package handlers_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	apphandlers "github.com/pixelcop/clientshare/internal/handlers"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/storage"
	tenantctx "github.com/pixelcop/clientshare/internal/tenant"
	"gorm.io/gorm"
)

func newAuthRequest(method, path, token string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	return req
}

func createFileRecord(t *testing.T, env *fileTestEnv, folder *models.File, filename, content string) *models.File {
	t.Helper()
	store := storage.NewLocalStorage(env.storageRoot).Tenant(folder.TenantID)
	filePath := filepath.Join(folder.Path, filename)
	if _, err := store.Upload(filePath, strings.NewReader(content)); err != nil {
		t.Fatalf("failed to upload test file: %v", err)
	}
	file := &models.File{
		TenantID:   folder.TenantID,
		ClientID:   folder.ClientID,
		UserID:     &env.adminUser.ID,
		FolderID:   &folder.ID,
		Filename:   filename,
		Path:       filePath,
		Type:       "file",
		Size:       int64(len(content)),
		UploadedAt: time.Now(),
	}
	if err := env.db.Create(file).Error; err != nil {
		t.Fatalf("failed to create file record: %v", err)
	}
	return file
}

func tenantDiskPath(root, tenantID string, pathParts ...string) string {
	parts := append([]string{root, "tenants", tenantID}, pathParts...)
	return filepath.Join(parts...)
}

func TestFilesAuthRequired(t *testing.T) {
	env := setupFileTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/folders", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestListFolderFilesDefaultForCustomer(t *testing.T) {
	env := setupFileTestEnv(t)
	_ = createFileRecord(t, env, env.rootFolder, "one.txt", "hello")

	req := newAuthRequest(http.MethodGet, "/api/folders", env.clientToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Items []models.File `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(payload.Items))
	}
}

func TestListFolderFilesDefaultForLinkUser(t *testing.T) {
	env := setupFileTestEnv(t)
	_ = createFileRecord(t, env, env.rootFolder, "one.txt", "hello")

	req := newAuthRequest(http.MethodGet, "/api/folders", env.linkToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Items []models.File `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(payload.Items))
	}
}

func TestListFolderFilesAdminMissingFolder(t *testing.T) {
	env := setupFileTestEnv(t)

	req := newAuthRequest(http.MethodGet, "/api/folders", env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestListFolderFilesInvalidFolderID(t *testing.T) {
	env := setupFileTestEnv(t)

	req := newAuthRequest(http.MethodGet, "/api/folders/does-not-exist", env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestCreateFolderInvalidName(t *testing.T) {
	env := setupFileTestEnv(t)

	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID, env.adminToken, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateFolderForbiddenForClient(t *testing.T) {
	env := setupFileTestEnv(t)

	payload := `{"name":"Reports"}`
	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID, env.otherToken, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestCreateFolderSuccess(t *testing.T) {
	env := setupFileTestEnv(t)

	payload := `{"name":"Reports"}`
	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID, env.adminToken, strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var folder models.File
	if err := env.db.Where("client_id = ? AND filename = ? AND type = ?", env.client.ID, "Reports", "folder").First(&folder).Error; err != nil {
		t.Fatalf("expected folder record, got error: %v", err)
	}
	if _, err := os.Stat(tenantDiskPath(env.storageRoot, folder.TenantID, folder.Path)); err != nil {
		t.Fatalf("expected folder on disk, got error: %v", err)
	}
}

func TestUploadFilesInvalidForm(t *testing.T) {
	env := setupFileTestEnv(t)

	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, strings.NewReader("not-multipart"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUploadFilesNoFiles(t *testing.T) {
	env := setupFileTestEnv(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}
	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestUploadFilesForbiddenForClient(t *testing.T) {
	env := setupFileTestEnv(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "note.txt")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.otherToken, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestUploadFilesSuccessQueuesEmail(t *testing.T) {
	env := setupFileTestEnv(t)

	if err := env.db.Create(&models.SecureLink{
		ClientID:   env.client.ID,
		Token:      "test-link-token",
		Email:      "linkviewer@test.local",
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		AccessType: "read",
		CreatedBy:  env.adminUser.ID,
		CreatedAt:  time.Now(),
	}).Error; err != nil {
		t.Fatalf("failed to create secure link: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("notify", "true")
	part, err := writer.CreateFormFile("files", "note.txt")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("failed to write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	if len(env.emailQueue.items) == 0 {
		t.Fatalf("expected email to be queued")
	}
	queued := env.emailQueue.items[0]
	if !strings.Contains(queued.Recipient, "linkviewer@test.local") {
		t.Fatalf("expected secure link email in recipients, got %q", queued.Recipient)
	}

	var count int64
	if err := env.db.Model(&models.File{}).Where("type = ?", "file").Count(&count).Error; err != nil {
		t.Fatalf("failed to count files: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file record, got %d", count)
	}

	var uploaded models.File
	if err := env.db.Where("type = ?", "file").First(&uploaded).Error; err != nil {
		t.Fatalf("failed to load uploaded file record: %v", err)
	}
	expectedFileIDs := uploaded.ID
	if queued.FileIDsCSV != expectedFileIDs {
		t.Fatalf("expected queued file_ids_csv %q, got %q", expectedFileIDs, queued.FileIDsCSV)
	}
}

func TestUploadFilesSuccessMergesPendingAndNewFileNamesAndIDs(t *testing.T) {
	env := setupFileTestEnv(t)

	existing := createFileRecord(t, env, env.rootFolder, "already-there.txt", "existing")
	existingID := existing.ID
	if _, err := env.queue.Queue(email.Email{
		To:         "client@test.local",
		Subject:    "pending",
		Body:       "pending",
		ClientID:   &env.client.ID,
		FileIDsCSV: existingID,
	}, nil); err != nil {
		t.Fatalf("failed to seed pending queue item: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("notify", "true")
	for _, name := range []string{"new-one.txt", "new-two.txt"} {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("failed to create form file %s: %v", name, err)
		}
		if _, err := part.Write([]byte("hello")); err != nil {
			t.Fatalf("failed to write form file %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	if len(env.queue.items) != 2 {
		t.Fatalf("expected 2 queued emails (seed + merged), got %d", len(env.queue.items))
	}
	queued := env.queue.items[1]

	if queued.Subject != "ClientShare: 3 new files uploaded" {
		t.Fatalf("expected merged subject for 3 files, got %q", queued.Subject)
	}

	if !strings.Contains(queued.Body, "already-there.txt") ||
		!strings.Contains(queued.Body, "new-one.txt") ||
		!strings.Contains(queued.Body, "new-two.txt") {
		t.Fatalf("expected queued body to include all file names, got %q", queued.Body)
	}

	parts := strings.Split(queued.FileIDsCSV, ",")
	if len(parts) != 3 {
		t.Fatalf("expected 3 file ids in queued csv, got %d (%q)", len(parts), queued.FileIDsCSV)
	}
	if parts[0] != existingID {
		t.Fatalf("expected first file id to be existing pending id %q, got %q", existingID, parts[0])
	}
}

func TestUploadFilesSuccessAssignsSharedBatchIDForMultiFileActivity(t *testing.T) {
	env := setupFileTestEnv(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, name := range []string{"first-batch.txt", "second-batch.txt"} {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("failed to create form file %s: %v", name, err)
		}
		if _, err := part.Write([]byte("hello")); err != nil {
			t.Fatalf("failed to write form file %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	activities := make([]models.FileActivity, 0)
	if err := env.db.Where("action = ?", "upload").Order("created_at ASC, id ASC").Find(&activities).Error; err != nil {
		t.Fatalf("failed to load upload activities: %v", err)
	}
	if len(activities) != 2 {
		t.Fatalf("expected 2 upload activities, got %d", len(activities))
	}
	if activities[0].UploadBatchID == nil || strings.TrimSpace(*activities[0].UploadBatchID) == "" {
		t.Fatalf("expected first upload activity to have an upload batch id")
	}
	if activities[1].UploadBatchID == nil || *activities[1].UploadBatchID != *activities[0].UploadBatchID {
		t.Fatalf("expected upload activities to share a non-empty upload batch id")
	}
	if activities[0].FileID == nil || activities[1].FileID == nil {
		t.Fatalf("expected upload activities to retain file ids")
	}
	if activities[0].ID == activities[1].ID {
		t.Fatalf("expected separate upload activity rows per file")
	}
	if activities[0].FileID != nil && activities[1].FileID != nil && *activities[0].FileID == *activities[1].FileID {
		t.Fatalf("expected each upload activity row to point at a different file id")
	}
}

func TestDownloadFolderZipNotFound(t *testing.T) {
	env := setupFileTestEnv(t)

	req := newAuthRequest(http.MethodGet, "/api/folders/does-not-exist/download-zip", env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDownloadFolderZipSuccess(t *testing.T) {
	env := setupFileTestEnv(t)
	_ = createFileRecord(t, env, env.rootFolder, "one.txt", "hello")

	req := newAuthRequest(http.MethodGet, "/api/folders/"+env.rootFolder.ID+"/download-zip", env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if contentDisposition := resp.Header.Get("Content-Disposition"); !strings.Contains(contentDisposition, ".zip") {
		t.Fatalf("expected zip content disposition, got %q", contentDisposition)
	}
}

func TestDownloadFolderZipSuccessForSecureLink(t *testing.T) {
	env := setupFileTestEnv(t)
	_ = createFileRecord(t, env, env.rootFolder, "one.txt", "hello")

	req := newAuthRequest(http.MethodGet, "/api/folders/"+env.rootFolder.ID+"/download-zip", env.linkToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if contentDisposition := resp.Header.Get("Content-Disposition"); !strings.Contains(contentDisposition, ".zip") {
		t.Fatalf("expected zip content disposition, got %q", contentDisposition)
	}
}

func TestDownloadFileByPublicIDNotFound(t *testing.T) {
	env := setupFileTestEnv(t)

	req := newAuthRequest(http.MethodGet, "/api/files/does-not-exist/download", env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDownloadFileByPublicIDFolderType(t *testing.T) {
	env := setupFileTestEnv(t)

	req := newAuthRequest(http.MethodGet, "/api/files/"+env.rootFolder.ID+"/download", env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestDownloadFileByPublicIDForbidden(t *testing.T) {
	env := setupFileTestEnv(t)
	file := createFileRecord(t, env, env.rootFolder, "blocked.txt", "nope")

	req := newAuthRequest(http.MethodGet, "/api/files/"+file.ID+"/download", env.otherToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestDownloadFileByPublicIDSuccess(t *testing.T) {
	env := setupFileTestEnv(t)
	file := createFileRecord(t, env, env.rootFolder, "readme.txt", "hello")

	req := newAuthRequest(http.MethodGet, "/api/files/"+file.ID+"/download", env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("expected text/plain content type, got %q", contentType)
	}
}

func TestDownloadFileByPublicIDSuccessForSecureLink(t *testing.T) {
	env := setupFileTestEnv(t)
	file := createFileRecord(t, env, env.rootFolder, "readme.txt", "hello")

	req := newAuthRequest(http.MethodGet, "/api/files/"+file.ID+"/download", env.linkToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/plain") {
		t.Fatalf("expected text/plain content type, got %q", contentType)
	}
}

func TestDownloadFileByPublicIDRejectsTenantMismatchedFileContext(t *testing.T) {
	env := setupFileTestEnv(t)
	handler := apphandlers.NewFileHandler(env.db, storage.NewLocalStorage(env.storageRoot), nil, "", nil)
	foreignFile := &models.File{
		ID:       "file-foreign",
		TenantID: "01JPMT0NX5K8J9Q4S7V2W3X700",
		ClientID: env.client.ID,
		Filename: "readme.txt",
		Path:     filepath.Join(env.client.FolderPath, "readme.txt"),
		Type:     "file",
	}

	app := fiber.New()
	app.Get("/download", func(c fiber.Ctx) error {
		tenantctx.SetLocal(c, tenantctx.RequestContext{ID: env.client.TenantID, Slug: tenantctx.DefaultTenantSlug, Name: tenantctx.DefaultTenantName, Resolution: "test"})
		c.Locals("file", foreignFile)
		return handler.DownloadFileByPublicID(c)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/download", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteFileByPublicIDNotFound(t *testing.T) {
	env := setupFileTestEnv(t)

	req := newAuthRequest(http.MethodDelete, "/api/files/does-not-exist", env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteFileByPublicIDSuccess(t *testing.T) {
	env := setupFileTestEnv(t)
	file := createFileRecord(t, env, env.rootFolder, "gone.txt", "bye")

	req := newAuthRequest(http.MethodDelete, "/api/files/"+file.ID, env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	var updated models.File
	if err := env.db.Where("id = ?", file.ID).First(&updated).Error; err != nil {
		t.Fatalf("failed to load file: %v", err)
	}
	if updated.DeletedAt == nil {
		t.Fatalf("expected deleted_at to be set")
	}
}

func TestDeleteFileByPublicIDRollsBackWhenActivityInsertFails(t *testing.T) {
	env := setupFileTestEnv(t)
	file := createFileRecord(t, env, env.rootFolder, "rollback.txt", "bye")

	callbackName := "test:fail_file_activity_insert"
	if err := env.db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "file_activity" {
			tx.AddError(errors.New("forced file activity insert failure"))
		}
	}); err != nil {
		t.Fatalf("register create callback failed: %v", err)
	}
	defer env.db.Callback().Create().Remove(callbackName)

	req := newAuthRequest(http.MethodDelete, "/api/files/"+file.ID, env.adminToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}

	var updated models.File
	if err := env.db.Where("id = ?", file.ID).First(&updated).Error; err != nil {
		t.Fatalf("failed to load file after rollback: %v", err)
	}
	if updated.DeletedAt != nil {
		t.Fatalf("expected deleted_at to remain nil after rollback")
	}

	var activityCount int64
	if err := env.db.Model(&models.FileActivity{}).Where("file_id = ? AND action = ?", file.ID, "delete").Count(&activityCount).Error; err != nil {
		t.Fatalf("failed to count delete activities: %v", err)
	}
	if activityCount != 0 {
		t.Fatalf("expected no delete activity after rollback, got %d", activityCount)
	}
}
