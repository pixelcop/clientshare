package handlers

import (
	"context"
	"errors"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/pixelcop/clientshare/internal/middleware"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/pkg/utils"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
)

type FileHandler struct {
	db             *gorm.DB
	store          storage.Storage
	clientService  *services.ClientService
	fileService    *services.FileService
	feedService    *services.FeedService
	emailQueue     email.EmailQueue
	baseURL        string
	tenantSettings *services.TenantSettingsService
}

func NewFileHandler(db *gorm.DB, store storage.Storage, emailQueue email.EmailQueue, baseURL string, tenantSettings *services.TenantSettingsService) *FileHandler {
	return &FileHandler{
		db:             db,
		store:          store,
		clientService:  &services.ClientService{DB: db, Storage: store},
		fileService:    services.NewFileService(db),
		feedService:    services.NewFeedService(db),
		emailQueue:     emailQueue,
		baseURL:        baseURL,
		tenantSettings: tenantSettings,
	}
}

// RegisterFileRoutes sets up file endpoints
// emailQueue may be nil if email notifications are disabled
func RegisterFileRoutes(api fiber.Router, db *gorm.DB, store storage.Storage, emailQueue email.EmailQueue, baseURL string, tenantSettings *services.TenantSettingsService) {
	handler := NewFileHandler(db, store, emailQueue, baseURL, tenantSettings)
	api.Get("/folders", handler.ListFolderFiles)
	api.Post("/folders/exists", handler.CheckFileExists)

	secureFolder := api.Group("/folders/:id", middleware.UserCanAccessFile)
	secureFolder.Get("", handler.ListFolderFiles)
	secureFolder.Post("", handler.CreateFolder)
	secureFolder.Get("/breadcrumbs", handler.GetBreadcrumbs)
	secureFolder.Post("/exists", handler.CheckFileExists)
	secureFolder.Post("/upload", handler.UploadFiles)
	secureFolder.Get("/download-zip", handler.DownloadFolderZip)

	secureFile := api.Group("/files/:id", middleware.UserCanAccessFile)
	secureFile.Get("", handler.GetFileOrFolder)
	secureFile.Delete("", handler.DeleteFileByPublicID)
	secureFile.Get("/download", handler.DownloadFileByPublicID)
}

func (h *FileHandler) siteTitleForTenant(c fiber.Ctx, tenantID string) string {
	if h.tenantSettings == nil {
		return services.DefaultSiteTitle
	}
	settings, err := h.tenantSettings.Get(c.Context(), tenantID)
	if err != nil {
		utils.Logger(c).Warn("failed loading tenant settings for upload notification", zap.Error(err), zap.String("tenant_id", tenantID))
		return services.DefaultSiteTitle
	}
	return services.SiteTitleOrDefault(settings)
}

func (h *FileHandler) storeForTenant(tenantID string) storage.Storage {
	return h.store.Tenant(tenantID)
}

func fileLocalForTenant(c fiber.Ctx) (*models.File, *fiber.Error) {
	file := fiber.Locals[*models.File](c, "file")
	if file == nil {
		return nil, nil
	}
	tenantID := tenantIDFromCtx(c)
	if strings.TrimSpace(file.TenantID) != "" && file.TenantID != tenantID {
		utils.Logger(c).Warn("tenant mismatch for file context", zap.String("request_tenant_id", tenantID), zap.String("file_tenant_id", file.TenantID), zap.String("file_id", file.ID))
		return nil, fiber.ErrNotFound
	}
	return file, nil
}

func (h *FileHandler) CheckFileExists(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	folder, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if folder == nil {
		var err *fiber.Error
		folder, err = h.getDefaultFolder(c)
		if err != nil {
			return c.Status(err.Code).JSON(fiber.Map{"error": err.Error()})
		}
	}
	if folder.Type != "folder" {
		utils.Logger(c).Debug("invalid folder type", zap.String("type", folder.Type))
		return c.Status(400).JSON(fiber.Map{"error": "invalid folder type"})
	}

	var body struct {
		Names []string `json:"names"`
	}
	if err := c.Bind().Body(&body); err != nil || len(body.Names) == 0 {
		utils.Logger(c).Debug("invalid names payload", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid names payload"})
	}

	existing, err := h.fileService.ListExistingNamesInFolder(c.Context(), tenantID, folder.ClientID, folder.ID, body.Names)
	if err != nil {
		utils.Logger(c).Error("error checking existing files", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"existing": existing})
}

func createFileActivity(ctx context.Context, db *gorm.DB, tenantID, clientID string, fileID *string, userID *string, linkID *string, uploadBatchID *string, action, filePath string) (*models.FileActivity, error) {
	activity := models.FileActivity{
		TenantID:      tenantID,
		ClientID:      clientID,
		FileID:        fileID,
		UserID:        userID,
		SecureLinkID:  linkID,
		UploadBatchID: uploadBatchID,
		Action:        action,
		FilePath:      filePath,
	}
	if err := db.WithContext(ctx).Create(&activity).Error; err != nil {
		return nil, err
	}
	return &activity, nil
}

func logFileActivity(ctx context.Context, db *gorm.DB, tenantID, clientID string, fileID *string, userID *string, linkID *string, action, filePath string) {
	_, _ = createFileActivity(ctx, db, tenantID, clientID, fileID, userID, linkID, nil, action, filePath)
}

func (h *FileHandler) deleteFileWithActivity(ctx context.Context, tenantID string, file *models.File) error {
	if file == nil {
		return gorm.ErrRecordNotFound
	}
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fileService := services.NewFileService(tx)
		if err := fileService.SoftDeleteFile(ctx, tenantID, file); err != nil {
			return err
		}
		_, err := createFileActivity(ctx, tx, tenantID, file.ClientID, &file.ID, file.UserID, nil, nil, "delete", file.Path)
		return err
	})
}

func (h *FileHandler) recordUploadFeedEvents(c fiber.Ctx, activities []*models.FileActivity) {
	for _, activity := range activities {
		if activity == nil {
			continue
		}
		if feedErr := h.feedService.RecordFileUploaded(c.Context(), activity); feedErr != nil {
			utils.Logger(c).Warn("error recording feed event", zap.Error(feedErr), zap.String("activity_id", activity.ID))
		}
	}
}

func notificationRecipients(ctx context.Context, db *gorm.DB, tenantID, role string, clientID string) ([]string, error) {
	var users []models.User
	recipients := make([]string, 0)
	seen := map[string]struct{}{}
	appendRecipient := func(emailAddr string) {
		emailAddr = strings.TrimSpace(emailAddr)
		if emailAddr == "" {
			return
		}
		key := strings.ToLower(emailAddr)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		recipients = append(recipients, emailAddr)
	}

	switch role {
	case "admin", "manager":
		if err := db.WithContext(ctx).
			Table("users").
			Joins("JOIN user_clients uc ON uc.tenant_id = users.tenant_id AND uc.user_id = users.id").
			Where("users.tenant_id = ? AND users.role = ? AND uc.tenant_id = ? AND uc.client_id = ?", tenantID, "client", tenantID, clientID).
			Distinct().
			Find(&users).Error; err != nil {
			return nil, err
		}
		var secureLinks []models.SecureLink
		if err := db.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND email <> ''", tenantID, clientID).Find(&secureLinks).Error; err != nil {
			return nil, err
		}
		for _, link := range secureLinks {
			appendRecipient(link.Email)
		}
	case "client", "customer", "link":
		if err := db.WithContext(ctx).Where("tenant_id = ? AND role IN ?", tenantID, []string{"admin", "manager"}).Find(&users).Error; err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	for _, user := range users {
		appendRecipient(user.Email)
	}
	return recipients, nil
}

func (h *FileHandler) getDefaultFolder(c fiber.Ctx) (*models.File, *fiber.Error) {
	role := fiber.Locals[string](c, "role")
	tenantID := tenantIDFromCtx(c)
	folderID := c.Params("id")
	if folderID == "" && !(role == "admin" || role == "manager") {
		// for customer/link users, pick a default client root folder they can access
		clientID := fiber.Locals[string](c, "client_id")
		if clientID == "" && role != "link" {
			userID := fiber.Locals[string](c, "user_id")
			var client models.Client
			if err := h.db.WithContext(c.Context()).
				Table("clients").
				Joins("JOIN user_clients uc ON uc.tenant_id = clients.tenant_id AND uc.client_id = clients.id").
				Where("clients.tenant_id = ? AND uc.tenant_id = ? AND uc.user_id = ?", tenantID, tenantID, userID).
				Order("clients.created_at ASC").
				First(&client).Error; err != nil {
				utils.Logger(c).Debug("user has no client assignments", zap.String("user_id", userID), zap.String("role", role), zap.Error(err))
				return nil, fiber.ErrNotFound
			}
			clientID = client.ID
		}
		if clientID == "" {
			utils.Logger(c).Debug("user has no client id", zap.String("role", role))
			return nil, fiber.ErrNotFound
		}
		client, err := h.clientService.GetClient(c.Context(), tenantID, clientID)
		if err != nil {
			utils.Logger(c).Debug("client not found for folder", zap.Error(err))
			return nil, fiber.ErrNotFound
		}
		folderID = derefString(client.RootFolderID)
	}

	if folderID == "" {
		// admin/manager with no folder specified? return error for now, could list all client root folders?
		utils.Logger(c).Debug("missing folder id")
		return nil, fiber.ErrBadRequest
	}

	if folderID != "" {
		// load folder by public id
		folder, err := h.fileService.GetFolderByPublicID(c.Context(), tenantID, folderID)
		if err != nil {
			utils.Logger(c).Debug("folder not found", zap.Error(err))
			return nil, fiber.ErrNotFound
		}
		return folder, nil
	}

	return nil, fiber.ErrInternalServerError
}

func (h *FileHandler) GetBreadcrumbs(c fiber.Ctx) error {
	folder, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if folder == nil {
		utils.Logger(c).Debug("folder not found for breadcrumbs")
		return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
	}

	hierarchy, err := h.fileService.GetFolderHierarchyByID(c.Context(), tenantIDFromCtx(c), folder.ClientID, folder.ID)
	if err != nil {
		utils.Logger(c).Error("error getting folder hierarchy", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(hierarchy)
}

func (h *FileHandler) ListFolderFiles(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	folder, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if folder == nil {
		var err *fiber.Error
		folder, err = h.getDefaultFolder(c)
		if err != nil {
			return c.Status(err.Code).JSON(fiber.Map{"error": err.Error()})
		}
	}

	fileType := fiber.Query[string](c, "type", "file")
	page, pageSize := getPageParams(c)
	sortBy := getSortBy(c)
	sortDirection := getSortDirection(c)
	pagination, dbFiles, err := h.fileService.ListFilesInFolder(c.Context(), tenantID, folder.ClientID, folder.ID, fileType, page, pageSize, sortBy, sortDirection)
	if err != nil {
		utils.Logger(c).Error("db error counting files", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	userID := fiber.Locals[string](c, "user_id")
	logFileActivity(c.Context(), h.db, tenantID, folder.ClientID, nil, &userID, nil, "list_files", "")
	return c.JSON(fiber.Map{
		"pagination": pagination,
		"items":      dbFiles,
	})
}

func (h *FileHandler) GetFileOrFolder(c fiber.Ctx) error {
	file, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if file == nil {
		utils.Logger(c).Debug("file not found")
		return c.Status(404).JSON(fiber.Map{"error": "file not found"})
	}
	return c.JSON(file)
}

// Pagination parameters
func getPageParams(c fiber.Ctx) (page, pageSize int) {
	page = fiber.Query[int](c, "page", 1)
	pageSize = fiber.Query[int](c, "page_size", 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return
}

func getSortBy(c fiber.Ctx) string {
	switch strings.ToLower(strings.TrimSpace(fiber.Query[string](c, "sort_by", "uploaded_at"))) {
	case "filename":
		return "filename"
	case "size":
		return "size"
	default:
		return "uploaded_at"
	}
}

func getSortDirection(c fiber.Ctx) string {
	if strings.EqualFold(strings.TrimSpace(fiber.Query[string](c, "sort_dir", "desc")), "asc") {
		return "asc"
	}
	return "desc"
}

// UploadFiles handles POST /api/folder/:id/upload
func (h *FileHandler) UploadFiles(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	folder, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if folder == nil {
		utils.Logger(c).Debug("folder not found")
		return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
	}
	if folder.Type != "folder" {
		utils.Logger(c).Debug("invalid folder type", zap.String("type", folder.Type))
		return c.Status(400).JSON(fiber.Map{"error": "invalid folder type"})
	}

	var err error
	var client *models.Client
	if client, err = h.clientService.GetClient(c.Context(), tenantID, folder.ClientID); err != nil {
		utils.Logger(c).Debug("client not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "client not found"})
	}

	form, err := c.MultipartForm()
	if err != nil || form == nil {
		utils.Logger(c).Debug("invalid multipart form", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid multipart form"})
	}

	files := form.File["files"]
	if len(files) == 0 {
		utils.Logger(c).Debug("no file uploaded")
		return c.Status(400).JSON(fiber.Map{"error": "no file uploaded"})
	}

	uploadedFiles := make([]models.File, 0, len(files))
	activities := make([]*models.FileActivity, 0, len(files))
	var uploadBatchID *string
	if len(files) > 1 {
		batchID := ids.NewULID()
		uploadBatchID = &batchID
	}

	tenantStore := h.storeForTenant(tenantID)
	userID := fiber.Locals[string](c, "user_id")
	for _, file := range files {
		f, err := file.Open()
		if err != nil {
			utils.Logger(c).Error("error opening file", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer f.Close()
		dstPath := filepath.Join(folder.Path, file.Filename)
		uploadedSize, err := tenantStore.Upload(dstPath, f)
		if err != nil {
			if errors.Is(err, os.ErrExist) || errors.Is(err, syscall.EEXIST) {
				utils.Logger(c).Debug("file already exists", zap.String("filename", file.Filename))
				return c.Status(409).JSON(fiber.Map{
					"error": "file already exists",
					"code":  "FILE_EXISTS",
					"field": "filename",
					"name":  file.Filename,
				})
			}
			utils.Logger(c).Error("error uploading file", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		// Store file metadata in DB
		fileRecord := &models.File{
			TenantID:   tenantID,
			ClientID:   client.ID,
			UserID:     &userID,
			FolderID:   &folder.ID,
			Filename:   file.Filename,
			Path:       dstPath,
			Type:       "file",
			Size:       uploadedSize,
			UploadedAt: time.Now(),
		}
		if fileRecord, err = h.fileService.CreateFileRecord(c.Context(), fileRecord); err != nil {
			// TODO: rollback file upload?
			utils.Logger(c).Error("error saving file metadata", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		utils.Logger(c).Info("file uploaded", zap.String("filename", file.Filename), zap.String("client_id", client.ID), zap.Int64("size", uploadedSize))
		activity, activityErr := createFileActivity(c.Context(), h.db, tenantID, client.ID, &fileRecord.ID, &userID, nil, uploadBatchID, "upload", dstPath)
		if activityErr != nil {
			utils.Logger(c).Warn("error recording file activity", zap.Error(activityErr))
		} else {
			activities = append(activities, activity)
			if uploadBatchID == nil {
				if feedErr := h.feedService.RecordFileUploaded(c.Context(), activity); feedErr != nil {
					utils.Logger(c).Warn("error recording feed event", zap.Error(feedErr))
				}
			}
		}
		uploadedFiles = append(uploadedFiles, *fileRecord)
	}

	if uploadBatchID != nil {
		switch {
		case len(activities) == len(uploadedFiles) && len(activities) > 0:
			if feedErr := h.feedService.RecordFilesUploaded(c.Context(), activities); feedErr != nil {
				utils.Logger(c).Warn("error recording grouped feed event", zap.Error(feedErr), zap.String("upload_batch_id", *uploadBatchID))
				h.recordUploadFeedEvents(c, activities)
			}
		case len(activities) > 0:
			utils.Logger(c).Warn("falling back to per-file feed events because upload activity recording was incomplete", zap.String("upload_batch_id", *uploadBatchID), zap.Int("activity_count", len(activities)), zap.Int("uploaded_file_count", len(uploadedFiles)))
			h.recordUploadFeedEvents(c, activities)
		}
	}

	h.sendUploadNotification(c, client, uploadedFiles)

	return c.SendStatus(201)
}

func (h *FileHandler) sendUploadNotification(c fiber.Ctx, client *models.Client, uploadedFiles []models.File) {
	notifyClient, _ := strconv.ParseBool(strings.ToLower(c.FormValue("notify", "true")))
	tenantID := tenantIDFromCtx(c)
	var recipientsTo string
	if h.emailQueue != nil && notifyClient {
		role := fiber.Locals[string](c, "role")
		recipients, err := notificationRecipients(c.Context(), h.db, tenantID, role, client.ID)
		if err != nil {
			utils.Logger(c).Error("error fetching email recipients", zap.Error(err))
		} else if len(recipients) > 0 {
			recipientsTo = strings.Join(recipients, ", ")
		}
	}

	// queue email after all files
	if h.emailQueue != nil && notifyClient && recipientsTo != "" {
		var fileIDs []string
		pending := h.emailQueue.Pending(tenantID, client.ID)
		if len(pending) > 0 && pending[0].FileIDsCSV != "" {
			fileIDs = strings.Split(pending[0].FileIDsCSV, ",")
		}
		notificationFiles := make([]models.File, 0, len(uploadedFiles)+len(fileIDs))
		for _, idStr := range fileIDs {
			idStr = strings.TrimSpace(idStr)
			if idStr == "" {
				continue
			}
			f, err := h.fileService.GetFileByID(c.Context(), tenantID, idStr)
			if err != nil {
				continue
			}
			notificationFiles = append(notificationFiles, *f)
		}

		for _, f := range uploadedFiles {
			fileIDs = append(fileIDs, f.ID)
			notificationFiles = append(notificationFiles, f)
		}

		subject, body, htmlBody := email.RenderUploadNotification(client.Name, notificationFiles, tenantBaseURLFromCtx(c, h.baseURL), h.siteTitleForTenant(c, tenantID))
		if _, queueErr := h.emailQueue.Queue(email.Email{
			TenantID:   tenantID,
			To:         recipientsTo,
			Subject:    subject,
			Body:       body,
			HTMLBody:   htmlBody,
			ClientID:   &client.ID,
			FileIDsCSV: strings.Join(fileIDs, ","),
		}, nil); queueErr != nil {
			utils.Logger(c).Error("error queueing upload email", zap.Error(queueErr))
		}
	}

}

// CreateFolder handles POST /api/folder/:id
func (h *FileHandler) CreateFolder(c fiber.Ctx) error {
	parentFolder, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if parentFolder == nil {
		utils.Logger(c).Debug("folder not found")
		return c.Status(404).JSON(fiber.Map{"error": "folder not found"})
	}
	if parentFolder.Type != "folder" {
		utils.Logger(c).Debug("invalid folder type", zap.String("type", parentFolder.Type))
		return c.Status(400).JSON(fiber.Map{"error": "invalid folder type"})
	}

	type req struct {
		Name string `json:"name"`
	}
	var body req
	if err := c.Bind().Body(&body); err != nil || body.Name == "" {
		utils.Logger(c).Debug("invalid folder name", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid folder name"})
	}
	subPath, err := resolveClientPath(parentFolder.Path, body.Name)
	if err != nil {
		utils.Logger(c).Debug("invalid folder path", zap.Error(err))
		return c.Status(400).JSON(fiber.Map{"error": "invalid folder name"})
	}
	if err := h.storeForTenant(tenantIDFromCtx(c)).CreateFolder(subPath); err != nil {
		utils.Logger(c).Error("error creating folder", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	userID := fiber.Locals[string](c, "user_id")
	folderRecord := &models.File{
		TenantID:   tenantIDFromCtx(c),
		ClientID:   parentFolder.ClientID,
		UserID:     &userID,
		FolderID:   &parentFolder.ID,
		Filename:   filepath.Base(subPath),
		Path:       subPath,
		Type:       "folder",
		Size:       0,
		UploadedAt: time.Now(),
	}
	if folderRecord, err = h.fileService.CreateFolderRecord(c.Context(), folderRecord); err != nil {
		utils.Logger(c).Error("error saving folder metadata", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	logFileActivity(c.Context(), h.db, tenantIDFromCtx(c), parentFolder.ClientID, &folderRecord.ID, &userID, nil, "create_folder", subPath)
	return c.JSON(folderRecord)
}

// DownloadZip handles GET /api/clients/:id/download-zip/*
func (h *FileHandler) DownloadFolderZip(c fiber.Ctx) error {
	file, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if file == nil {
		utils.Logger(c).Debug("file not found")
		return c.Status(404).JSON(fiber.Map{"error": "file not found"})
	}

	if file.Type != "folder" {
		utils.Logger(c).Debug("invalid file type")
		return c.Status(400).JSON(fiber.Map{"error": "invalid file type"})
	}

	tmpZip := filepath.Join(os.TempDir(), "clientshare_zip_"+file.Filename+".zip")
	if err := h.storeForTenant(tenantIDFromCtx(c)).ZipFolder(file.Path, tmpZip); err != nil {
		utils.Logger(c).Error("error zipping folder", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer os.Remove(tmpZip)

	userID, _ := c.Locals("user_id").(string)
	logFileActivity(c.Context(), h.db, tenantIDFromCtx(c), file.ClientID, &file.ID, &userID, nil, "download_zip", file.Path)
	c.Set("Content-Disposition", "attachment; filename="+file.Filename+".zip")
	return c.SendFile(tmpZip)
}

// DownloadFileByPublicID handles GET /api/files/:id/download
func (h *FileHandler) DownloadFileByPublicID(c fiber.Ctx) error {
	file, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if file == nil {
		utils.Logger(c).Debug("file not found")
		return c.Status(404).JSON(fiber.Map{"error": "file not found"})
	}

	if file.Type == "folder" {
		utils.Logger(c).Debug("invalid file type")
		return c.Status(400).JSON(fiber.Map{"error": "invalid file type"})
	}

	reader, err := h.storeForTenant(tenantIDFromCtx(c)).Download(file.Path)
	if err != nil {
		utils.Logger(c).Debug("file not found", zap.Error(err))
		return c.Status(404).JSON(fiber.Map{"error": "file not found"})
	}
	defer reader.Close()
	logFileActivity(c.Context(), h.db, tenantIDFromCtx(c), file.ClientID, &file.ID, file.UserID, nil, "download", file.Path)
	// Set Content-Type based on file extension
	ext := filepath.Ext(file.Filename)
	contentType := "application/octet-stream"
	if ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			contentType = t
		}
	}
	c.Set("Content-Type", contentType)
	c.Set("Content-Disposition", "attachment; filename="+file.Filename)
	c.RequestCtx().SetBodyStream(reader, -1)
	return c.SendStatus(200)
}

// DeleteFileByPublicID handles DELETE /api/files/:id
func (h *FileHandler) DeleteFileByPublicID(c fiber.Ctx) error {
	file, tenantErr := fileLocalForTenant(c)
	if tenantErr != nil {
		return c.Status(tenantErr.Code).JSON(fiber.Map{"error": tenantErr.Error()})
	}
	if file == nil {
		utils.Logger(c).Debug("file not found")
		return c.Status(404).JSON(fiber.Map{"error": "file not found"})
	}

	if err := h.deleteFileWithActivity(c.Context(), tenantIDFromCtx(c), file); err != nil {
		utils.Logger(c).Error("error soft deleting file", zap.Error(err))
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
