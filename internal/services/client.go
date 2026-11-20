package services

import (
	"context"
	"errors"
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
	if err := LogicalName(name); err != nil {
		return "", err
	}
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

		rootFolder := &models.File{
			TenantID:   tenantID,
			ClientID:   client.ID,
			Filename:   client.FolderPath,
			Path:       client.FolderPath,
			Type:       "folder",
			Size:       0,
			UploadedAt: time.Now().UTC(),
		}
		if err := tx.Create(rootFolder).Error; err != nil {
			return err
		}
		client.RootFolderID = &rootFolder.ID
		if err := tx.Save(client).Error; err != nil {
			return err
		}
		return models.DirtyView(tx, tenantID, client.ID)
	})
	if err != nil {
		return nil, err
	}
	return client, nil
}

// CreateClientForActor preserves manager client creation without turning it
// into tenant-wide authority: the creator is assigned the newly created client
// in the same transaction. Role strings from the request are never trusted.
func (s *ClientService) CreateClientForActor(ctx context.Context, tenantID, userID, name string) (*models.Client, error) {
	var client *models.Client
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockTenantAccess(tx, tenantID); err != nil {
			return err
		}
		var user models.User
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, userID).First(&user).Error; err != nil {
			return err
		}
		if user.Role != "admin" && user.Role != "manager" {
			return gorm.ErrRecordNotFound
		}
		profile, err := AccessProfileFor(tx, user)
		if err != nil {
			return err
		}
		if user.Role != "admin" && !profile.CanManageUsers {
			return gorm.ErrRecordNotFound
		}
		client, err = (&ClientService{DB: tx, Storage: s.Storage}).CreateClient(ctx, tenantID, name)
		if err != nil {
			return err
		}
		if user.Role == "manager" {
			return tx.Create(&models.UserClient{TenantID: tenantID, UserID: user.ID, ClientID: client.ID}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	items := []models.Client{*client}
	if err := s.decorateClientAccess(ctx, tenantID, userID, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

// ListClients returns all clients for admins and assigned clients for non-admin users.
func (s *ClientService) ListClients(ctx context.Context, tenantID, userID, role string) ([]models.Client, error) {
	q, err := s.clientListQuery(ctx, tenantID, userID, role, "")
	if err != nil {
		return nil, err
	}

	var clients []models.Client
	if q == nil {
		return clients, nil
	}
	if err := q.Order("clients.name ASC").Find(&clients).Error; err != nil {
		return nil, err
	}
	if err := s.decorateClientAccess(ctx, tenantID, userID, clients); err != nil {
		return nil, err
	}
	return clients, nil
}

// ListClientsPaginated returns a paginated list of clients, optionally filtered by search.
func (s *ClientService) ListClientsPaginated(ctx context.Context, tenantID, userID, role, search string, page, pageSize int) (*models.PaginationResponse, []models.Client, error) {
	page = normalizePage(page)
	pageSize = normalizePageSize(pageSize)

	q, err := s.clientListQuery(ctx, tenantID, userID, role, search)
	if err != nil {
		return nil, nil, err
	}
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
	if err := s.decorateClientAccess(ctx, tenantID, userID, clients); err != nil {
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

func (s *ClientService) clientListQuery(ctx context.Context, tenantID, userID, _ string, search string) (*gorm.DB, error) {
	tenantID, err := requireTenantID(tenantID)
	if err != nil {
		return nil, err
	}
	q, err := (&AccessService{DB: s.DB}).ClientScope(ctx, tenantID, userID, AccessContext)
	if err != nil {
		return nil, err
	}

	search = strings.TrimSpace(search)
	if search != "" {
		like := "%" + search + "%"
		q = q.Where("clients.name LIKE ? OR clients.folder_path LIKE ?", like, like)
	}

	return q, nil
}

func (s *ClientService) decorateClientAccess(ctx context.Context, tenantID, userID string, clients []models.Client) error {
	for start := 0; start < len(clients); start += maxAccessItems {
		batch := clients[start:min(start+maxAccessItems, len(clients))]
		ids := make([]string, len(batch))
		for i := range batch {
			ids[i] = batch[i].ID
		}
		decisions, err := (&AccessService{DB: s.DB}).ResolveClients(ctx, tenantID, userID, ids, AccessContext)
		if err != nil {
			return err
		}
		for i := range batch {
			d := decisions[batch[i].ID]
			if !d.Allowed {
				return gorm.ErrRecordNotFound
			}
			batch[i].Capabilities = &d.Capabilities
			manage := d.CanManageClient
			batch[i].CanManage = &manage
		}
	}
	return nil
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
		if strings.TrimSpace(newName) == client.Name {
			return nil
		}
		sanitized, err := s.sanitizeFolderName(ctx, tx, tenantID, newName)
		if err != nil {
			return err
		}

		client.Name = sanitized
		client.FolderPath = sanitized
		if err := tx.Save(&client).Error; err != nil {
			return err
		}
		if client.RootFolderID != nil {
			if err := tx.Model(&models.File{}).Where("tenant_id = ? AND id = ?", tenantID, *client.RootFolderID).Update("filename", sanitized).Error; err != nil {
				return err
			}
		}
		if err := RefreshLogicalPaths(tx, tenantID, client.ID); err != nil {
			return err
		}
		return models.DirtyView(tx, tenantID, client.ID)
	})
	if err != nil {
		return nil, err
	}
	return &client, nil
}

// DeleteClient retains file records and immutable objects for the retention period.
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

		if err := tx.Model(&models.File{}).Where("tenant_id = ? AND client_id = ? AND deleted_at IS NULL", tenantID, id).Update("deleted_at", time.Now().UTC()).Error; err != nil {
			return err
		}
		if err := tx.Where("tenant_id = ? AND id = ?", tenantID, id).Delete(&models.Client{}).Error; err != nil {
			return err
		}
		return models.DirtyView(tx, tenantID, id)
	})
}
