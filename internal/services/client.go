package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"gorm.io/gorm"
)

// ClientService handles business logic for clients and folder management
// Assumes storage root is provided externally (from config)
type ClientService struct {
	DB      *gorm.DB
	Storage storage.Storage
}

// SanitizeFolderName ensures folder name is safe and unique
func (s *ClientService) SanitizeFolderName(ctx context.Context, tenantID, name string) (string, error) {
	return s.sanitizeFolderName(ctx, s.DB, tenantID, name)
}

func (s *ClientService) sanitizeFolderName(ctx context.Context, db *gorm.DB, tenantID, name string) (string, error) {
	if _, err := requireTenantID(tenantID); err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("folder name cannot be empty")
	}
	if strings.ContainsAny(name, "/\\:") {
		return "", errors.New("folder name contains invalid characters")
	}

	// Check for reserved names
	reservedNames := []string{"trash", "tmp", "temp", "system", "admin", "config", "storage"}
	lowerName := strings.ToLower(name)
	for _, reserved := range reservedNames {
		if lowerName == reserved {
			return "", errors.New("folder name is reserved for internal use")
		}
	}

	// Uniqueness check
	var count int64
	if err := db.WithContext(ctx).Model(&models.Client{}).Where("tenant_id = ? AND name = ?", tenantID, name).Count(&count).Error; err != nil {
		return "", err
	}
	if count > 0 {
		return "", errors.New("folder name must be unique")
	}
	return name, nil
}

// CreateClient creates a client and folder
func (s *ClientService) CreateClient(ctx context.Context, tenantID, name string) (*models.Client, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	tenantStore := s.Storage.Tenant(tenantID)
	sanitized, err := s.SanitizeFolderName(ctx, tenantID, name)
	if err != nil {
		return nil, err
	}

	folderPath := sanitized
	client := &models.Client{TenantID: tenantID, Name: sanitized, FolderPath: folderPath}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(client).Error; err != nil {
			return err
		}
		if err := tenantStore.CreateFolder(storage.ClientFolderPath(folderPath)); err != nil {
			return err
		}

		rootFolder := &models.File{
			TenantID:   tenantID,
			ClientID:   client.ID,
			Filename:   client.FolderPath,
			Path:       client.FolderPath,
			Type:       "folder",
			Size:       0,
			UploadedAt: time.Now(),
		}
		if err := tx.Create(rootFolder).Error; err != nil {
			return err
		}
		client.RootFolderID = &rootFolder.ID
		return tx.Save(client).Error
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

// ListClients returns all clients for admins and assigned clients for non-admin users.
func (s *ClientService) ListClients(ctx context.Context, tenantID, userID, role string) ([]models.Client, error) {
	q := s.clientListQuery(ctx, tenantID, userID, role, "")

	var clients []models.Client
	if q == nil {
		return clients, nil
	}
	if err := q.Order("clients.name ASC").Find(&clients).Error; err != nil {
		return nil, err
	}
	return clients, nil
}

// ListClientsPaginated returns a paginated list of clients, optionally filtered by search.
func (s *ClientService) ListClientsPaginated(ctx context.Context, tenantID, userID, role, search string, page, pageSize int) (*models.PaginationResponse, []models.Client, error) {
	page = normalizePage(page)
	pageSize = normalizePageSize(pageSize)

	q := s.clientListQuery(ctx, tenantID, userID, role, search)
	if q == nil {
		pagination := models.PaginationResponse{
			TotalItems:   0,
			TotalPages:   0,
			Page:         page,
			PageSize:     pageSize,
			HasMore:      false,
			ItemsPerPage: 0,
		}
		return &pagination, []models.Client{}, nil
	}

	var totalItems int64
	if err := q.Count(&totalItems).Error; err != nil {
		return nil, nil, err
	}

	totalPages := 0
	if totalItems > 0 {
		totalPages = int(math.Ceil(float64(totalItems) / float64(pageSize)))
		if page > totalPages {
			page = totalPages
		}
	}

	var clients []models.Client
	offset := (page - 1) * pageSize
	if err := q.Order("clients.name ASC").Limit(pageSize).Offset(offset).Find(&clients).Error; err != nil {
		return nil, nil, err
	}

	pagination := models.PaginationResponse{
		TotalItems:   totalItems,
		TotalPages:   totalPages,
		Page:         page,
		PageSize:     pageSize,
		HasMore:      page < totalPages,
		ItemsPerPage: len(clients),
	}

	return &pagination, clients, nil
}

func (s *ClientService) clientListQuery(ctx context.Context, tenantID, userID, role, search string) *gorm.DB {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil
	}
	q := s.DB.WithContext(ctx).Table("clients").Where("clients.tenant_id = ?", tenantID).Distinct()
	if role != "admin" && role != "manager" {
		if strings.TrimSpace(userID) == "" {
			return nil
		}
		q = q.Joins("JOIN user_clients uc ON uc.client_id = clients.id AND uc.tenant_id = clients.tenant_id").Where("uc.user_id = ? AND uc.tenant_id = ?", userID, tenantID)
	}

	search = strings.TrimSpace(search)
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("clients.name LIKE ? OR clients.folder_path LIKE ?", like, like)
	}

	return q
}

func normalizePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

func normalizePageSize(pageSize int) int {
	if pageSize < 1 {
		return 20
	}
	return pageSize
}

// GetClient returns client by ID
func (s *ClientService) GetClient(ctx context.Context, tenantID, id string) (*models.Client, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	var client models.Client
	if err := s.DB.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, id).First(&client).Error; err != nil {
		return nil, err
	}
	return &client, nil
}

// UpdateClient renames client and folder
func (s *ClientService) UpdateClient(ctx context.Context, tenantID, id string, newName string) (*models.Client, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	var client models.Client
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).First(&client).Error; err != nil {
			return err
		}
		tenantStore := s.Storage.Tenant(client.TenantID)
		sanitized, err := s.sanitizeFolderName(ctx, tx, tenantID, newName)
		if err != nil {
			return err
		}

		originalFolderPath := client.FolderPath
		if err := tenantStore.Move(storage.ClientFolderPath(originalFolderPath), storage.ClientFolderPath(sanitized)); err != nil {
			return err
		}

		client.Name = sanitized
		client.FolderPath = sanitized
		if err := tx.Save(&client).Error; err != nil {
			if rollbackErr := tenantStore.Move(storage.ClientFolderPath(sanitized), storage.ClientFolderPath(originalFolderPath)); rollbackErr != nil {
				return fmt.Errorf("update client after moving folder: %w (rollback move failed: %v)", err, rollbackErr)
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// DeleteClient moves folder to trash and deletes client
func (s *ClientService) DeleteClient(ctx context.Context, tenantID, id string) error {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var client models.Client
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).First(&client).Error; err != nil {
			return err
		}

		tenantStore := s.Storage.Tenant(tenantID)
		sourcePath := storage.ClientFolderPath(client.FolderPath)
		trashPath := storage.TrashPath(client.FolderPath)
		if err := tenantStore.Move(sourcePath, trashPath); err != nil {
			return err
		}

		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Client{}).Error; err != nil {
			if rollbackErr := tenantStore.Move(trashPath, sourcePath); rollbackErr != nil {
				return fmt.Errorf("delete client after moving folder to trash: %w (rollback move failed: %v)", err, rollbackErr)
			}
			return err
		}
		return nil
	})
}
