package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pixelcop/clientshare/internal/cache"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

// FileService centralizes file table queries.
type FileService struct {
	DB *gorm.DB

	folderHierarchyCache *cache.Cache[string, []models.File]
	cacheIndexMu         sync.Mutex
	clientCacheKeys      map[string]map[string]struct{}
}

func NewFileService(db *gorm.DB) *FileService {
	return &FileService{
		DB:                   db,
		folderHierarchyCache: cache.New[string, []models.File](),
		clientCacheKeys:      make(map[string]map[string]struct{}),
	}
}

func (s *FileService) GetFileByPublicID(ctx context.Context, tenantID, publicID string) (*models.File, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	if publicID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var file models.File
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, publicID).First(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *FileService) GetFolderByPublicID(ctx context.Context, tenantID, folderID string) (*models.File, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	if folderID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var folder models.File
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ? AND type = 'folder' AND deleted_at IS NULL", tenantID, folderID).First(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *FileService) GetFolderByClientPath(ctx context.Context, tenantID string, clientID string, folderPath string) (*models.File, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	var folder models.File
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND path = ? AND type = ? AND deleted_at IS NULL", tenantID, clientID, folderPath, "folder").First(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

func (s *FileService) GetFileByClientPath(ctx context.Context, tenantID string, clientID string, path string) (*models.File, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	var file models.File
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND path = ? AND deleted_at IS NULL", tenantID, clientID, path).First(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *FileService) GetOrCreateFolderRecord(ctx context.Context, tenantID string, client models.Client, userID *string, folderPath string) (*models.File, error) {
	folder, err := s.GetFolderByClientPath(ctx, tenantID, client.ID, folderPath)
	if err == nil {
		return folder, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	folder = &models.File{
		TenantID:   tenantID,
		ClientID:   client.ID,
		UserID:     userID,
		Filename:   filepath.Base(folderPath),
		Path:       folderPath,
		Type:       "folder",
		Size:       0,
		UploadedAt: time.Now(),
	}
	if folder, err = s.CreateFolderRecord(ctx, folder); err != nil {
		return nil, err
	}
	return folder, nil
}

func (s *FileService) CreateFileRecord(ctx context.Context, file *models.File) (*models.File, error) {
	if file == nil || strings.TrimSpace(file.TenantID) == "" {
		return nil, errors.New("tenant id is required")
	}
	err := s.DB.WithContext(ctx).Create(file).Error
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (s *FileService) CreateFolderRecord(ctx context.Context, folder *models.File) (*models.File, error) {
	if folder == nil || strings.TrimSpace(folder.TenantID) == "" {
		return nil, errors.New("tenant id is required")
	}
	err := s.DB.WithContext(ctx).Create(folder).Error
	if err != nil {
		return nil, err
	}
	s.invalidateFolderHierarchyCacheByClient(folder.ClientID)
	return folder, nil
}

func (s *FileService) GetFileByID(ctx context.Context, tenantID, id string) (*models.File, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	var file models.File
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ? AND deleted_at IS NULL", tenantID, id).First(&file).Error; err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *FileService) ListExistingNamesInFolder(ctx context.Context, tenantID string, clientID string, folderID string, names []string) ([]string, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return []string{}, nil
	}
	var existing []string
	err = s.DB.WithContext(ctx).Model(&models.File{}).
		Where("tenant_id = ? AND client_id = ? AND folder_id = ? AND deleted_at IS NULL AND filename IN ?", tenantID, clientID, folderID, names).
		Distinct("filename").
		Pluck("filename", &existing).Error
	if err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *FileService) ListFilesInFolder(ctx context.Context, tenantID string, clientID string, folderID string, fileType string, page, pageSize int, sortBy string, sortDirection string) (*models.PaginationResponse, []models.File, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, nil, err
	}
	q := s.DB.WithContext(ctx).Model(&models.File{}).Where("tenant_id = ? AND client_id = ? AND folder_id = ? AND type = ? AND deleted_at IS NULL", tenantID, clientID, folderID, fileType)

	var totalItems int64
	if err := q.Count(&totalItems).Error; err != nil {
		return nil, nil, err
	}

	// Calculate total pages
	totalPages := int(totalItems / int64(pageSize))
	if int(totalItems%int64(pageSize)) > 0 {
		totalPages++
	}

	var dbFiles []models.File
	offset := (page - 1) * pageSize
	orderDirection := "DESC"
	if strings.EqualFold(strings.TrimSpace(sortDirection), "asc") {
		orderDirection = "ASC"
	}

	orderBy := "uploaded_at " + orderDirection + ", id " + orderDirection
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "filename":
		orderBy = "filename " + orderDirection + ", id " + orderDirection
	case "size":
		orderBy = "size " + orderDirection + ", filename " + orderDirection + ", id " + orderDirection
	}

	if err := q.Order(orderBy).Limit(pageSize).Offset(offset).Find(&dbFiles).Error; err != nil {
		return nil, nil, err
	}

	pagination := models.PaginationResponse{
		TotalItems:   totalItems,
		TotalPages:   totalPages,
		Page:         page,
		PageSize:     pageSize,
		HasMore:      page < totalPages,
		ItemsPerPage: len(dbFiles),
	}

	return &pagination, dbFiles, nil
}

func (s *FileService) SoftDeleteFile(ctx context.Context, tenantID string, file *models.File) error {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return err
	}
	if file == nil {
		return gorm.ErrRecordNotFound
	}
	if err := s.DB.WithContext(ctx).Model(&models.File{}).Where("tenant_id = ? AND id = ?", tenantID, file.ID).Update("deleted_at", models.Now()).Error; err != nil {
		return err
	}
	s.invalidateFolderHierarchyCacheByClient(file.ClientID)
	return nil
}

// GetFolderHierarchyByID returns the starting folder and all its parent folders up to root.
// The result is ordered from root folder to the starting folder.
func (s *FileService) GetFolderHierarchyByID(ctx context.Context, tenantID, clientID string, folderID string) ([]models.File, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	if clientID == "" || folderID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	cacheKey := folderHierarchyCacheKey(tenantID, clientID, folderID)
	if cached, ok := s.folderHierarchyCache.Get(cacheKey); ok {
		return cloneFolderSlice(cached), nil
	}

	var folders []models.File
	err = s.DB.WithContext(ctx).Raw(`
		WITH RECURSIVE folder_hierarchy AS (
			SELECT
				tenant_id, id, client_id, user_id, folder_id, filename, path, type, size, uploaded_at, deleted_at,
				0 AS depth
			FROM files
			WHERE tenant_id = ? AND client_id = ? AND id = ? AND type = 'folder' AND deleted_at IS NULL

			UNION ALL

			SELECT
				f.tenant_id, f.id, f.client_id, f.user_id, f.folder_id, f.filename, f.path, f.type, f.size, f.uploaded_at, f.deleted_at,
				fh.depth + 1
			FROM files f
			JOIN folder_hierarchy fh ON fh.folder_id = f.id
			WHERE f.tenant_id = ? AND f.client_id = ? AND f.type = 'folder' AND f.deleted_at IS NULL
		)
		SELECT tenant_id, id, client_id, user_id, folder_id, filename, path, type, size, uploaded_at, deleted_at
		FROM folder_hierarchy
		ORDER BY depth DESC
	`, tenantID, clientID, folderID, tenantID, clientID).Scan(&folders).Error

	if err != nil {
		return nil, err
	}
	if len(folders) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	s.folderHierarchyCache.Set(cacheKey, cloneFolderSlice(folders))
	s.trackClientCacheKey(clientID, cacheKey)

	return folders, nil
}

func folderHierarchyCacheKey(tenantID string, clientID string, folderID string) string {
	return fmt.Sprintf("%s:%s:%s", tenantID, clientID, folderID)
}

func cloneFolderSlice(in []models.File) []models.File {
	if len(in) == 0 {
		return nil
	}
	out := make([]models.File, len(in))
	copy(out, in)
	return out
}

func (s *FileService) trackClientCacheKey(clientID string, key string) {
	s.cacheIndexMu.Lock()
	defer s.cacheIndexMu.Unlock()

	if s.clientCacheKeys[clientID] == nil {
		s.clientCacheKeys[clientID] = make(map[string]struct{})
	}
	s.clientCacheKeys[clientID][key] = struct{}{}
}

func (s *FileService) invalidateFolderHierarchyCacheByClient(clientID string) {
	s.cacheIndexMu.Lock()
	keys := s.clientCacheKeys[clientID]
	delete(s.clientCacheKeys, clientID)
	s.cacheIndexMu.Unlock()

	for key := range keys {
		s.folderHierarchyCache.Delete(key)
	}
}
