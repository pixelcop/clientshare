package services

import (
	"context"
	"crypto/hmac"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/links"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/pixelcop/clientshare/pkg/utils/ids"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	requestNeeded      = "needed"
	requestSubmitted   = "submitted"
	requestReplacement = "replacement"
	requestAccepted    = "accepted"
	requestMaxFiles    = 20
)

var (
	ErrDocumentRequestInput        = errors.New("invalid document request")
	ErrDocumentRequestConflict     = errors.New("this item has changed or has no available files; refresh and try again")
	ErrDocumentRequestFiles        = errors.New("an item can have at most 20 files per submission")
	ErrDocumentRequestFolder       = errors.New("enter a valid destination folder name without slashes")
	ErrDocumentRequestFolderExists = errors.New("a folder with that name already exists; choose a different destination folder name")
	ErrDocumentRequestRecipient    = errors.New("choose an existing client user with access to this workspace")
)

type DocumentRequestService struct {
	DB                  *gorm.DB
	Store               storage.Storage
	SigningKey          string
	NotificationBaseURL string
}

type DocumentRequestInput struct {
	Title          string                     `json:"title"`
	FolderName     string                     `json:"folder_name"`
	RecipientEmail string                     `json:"recipient_email"`
	Items          []DocumentRequestItemInput `json:"items"`
}

type DocumentRequestItemInput struct {
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
}

type DocumentRequestView struct {
	ID          string                    `json:"id,omitempty"`
	Title       string                    `json:"title"`
	ExpiresAt   time.Time                 `json:"expires_at"`
	Revoked     bool                      `json:"revoked,omitempty"`
	Link        string                    `json:"link,omitempty"`
	FolderID    string                    `json:"folder_id,omitempty"`
	FolderName  string                    `json:"folder_name,omitempty"`
	EmailStatus string                    `json:"email_status,omitempty"`
	Items       []DocumentRequestItemView `json:"items"`
}

type DocumentRequestItemView struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Instructions string                  `json:"instructions"`
	Status       string                  `json:"status"`
	Reason       string                  `json:"reason"`
	Round        int                     `json:"round"`
	Files        []DocumentRequestFile   `json:"files"`
	History      []DocumentRequestReview `json:"history,omitempty"`
}

type DocumentRequestFile struct {
	ID    string `json:"id,omitempty"` // present only for staff; ordinary file authorization still applies
	Name  string `json:"name"`
	Round int    `json:"round"`
}

type DocumentRequestReview struct {
	Round       int       `json:"round"`
	Status      string    `json:"status"`
	Reason      string    `json:"reason"`
	SubmittedAt time.Time `json:"submitted_at"`
	ReviewedAt  time.Time `json:"reviewed_at"`
}

func (s *DocumentRequestService) staff(ctx context.Context, tx *gorm.DB, req AccessRequest, create bool) error {
	req.FileID, req.Operation = "", AccessContext
	decision, err := (&AccessService{DB: tx}).Resolve(ctx, req)
	if err != nil {
		return err
	}
	if !decision.Allowed || !decision.CanManageClient || !decision.Capabilities.CanRead ||
		(create && !decision.Capabilities.CanUpload) {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *DocumentRequestService) token(tenantID, requestID string) string {
	return links.DocumentRequestToken(s.SigningKey, tenantID, requestID)
}

func (s *DocumentRequestService) Create(ctx context.Context, req AccessRequest, input DocumentRequestInput) (*DocumentRequestView, error) {
	input.Title = strings.TrimSpace(input.Title)
	if s.SigningKey == "" || input.Title == "" || utf8.RuneCountInString(input.Title) > 160 || len(input.Items) == 0 || len(input.Items) > 30 {
		return nil, ErrDocumentRequestInput
	}
	name := strings.TrimSpace(input.FolderName)
	if name == "" {
		name = input.Title
	}
	if len(name) > 255 || LogicalName(name) != nil || strings.ContainsAny(name, "\r\n") {
		return nil, ErrDocumentRequestFolder
	}
	address := strings.TrimSpace(input.RecipientEmail)
	emailStatus := ""
	row := models.DocumentRequest{
		ID: ids.NewULID(), TenantID: req.TenantID, ClientID: req.ClientID, Title: input.Title,
		CreatedBy: req.Actor.UserID, ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
	}
	items := make([]models.DocumentRequestItem, 0, len(input.Items))
	for position, item := range input.Items {
		item.Name, item.Instructions = strings.TrimSpace(item.Name), strings.TrimSpace(item.Instructions)
		if item.Name == "" || utf8.RuneCountInString(item.Name) > 160 || utf8.RuneCountInString(item.Instructions) > 1000 {
			return nil, ErrDocumentRequestInput
		}
		items = append(items, models.DocumentRequestItem{RequestID: row.ID, Position: position, Name: item.Name, Instructions: item.Instructions, Status: requestNeeded, Round: 1})
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockTenantAccess(tx, req.TenantID); err != nil {
			return err
		}
		if err := s.staff(ctx, tx, req, true); err != nil {
			return err
		}
		var recipient *models.User
		if address != "" {
			if email.CommentNotificationAddress(address) == "" {
				return ErrDocumentRequestRecipient
			}
			var users []models.User
			if err := tx.Where("tenant_id = ? AND LOWER(email) = ?", row.TenantID, strings.ToLower(address)).Limit(2).Find(&users).Error; err != nil {
				return err
			}
			if len(users) != 1 {
				return ErrDocumentRequestRecipient
			}
			allowed, err := email.DocumentRequestRecipient(ctx, tx, row.ClientID, users[0], time.Now())
			if err != nil {
				return err
			}
			if !allowed {
				return ErrDocumentRequestRecipient
			}
			recipient = &users[0]
		}
		var client models.Client
		if err := tx.Where("tenant_id = ? AND id = ?", row.TenantID, row.ClientID).First(&client).Error; err != nil {
			return err
		}
		if client.RootFolderID == nil {
			return gorm.ErrRecordNotFound
		}
		root, err := requestFolder(tx, row.TenantID, row.ClientID, *client.RootFolderID)
		if err != nil {
			return err
		}
		folder, err := (&FileService{DB: tx}).CreateFolderRecord(ctx, &models.File{
			TenantID: row.TenantID, ClientID: row.ClientID, UserID: &row.CreatedBy,
			FolderID: &root.ID, Filename: name, Path: filepath.Join(root.Path, name), Type: "folder",
		})
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return ErrDocumentRequestFolderExists
			}
			return err
		}
		row.FolderID = folder.ID
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		if recipient == nil {
			return nil
		}
		if s.NotificationBaseURL == "" {
			emailStatus = "unavailable"
			return nil
		}
		key := "document-request:" + row.ID
		emailStatus = "queued"
		return tx.Create(&models.QueuedEmail{
			TenantID: row.TenantID, ClientID: &row.ClientID, DocumentRequestID: &row.ID,
			Recipient: recipient.Email, RecipientUserID: &recipient.ID, Subject: "New document request",
			ScheduledFor: time.Now().UTC(), DeduplicationKey: &key,
			EventType: email.DocumentRequestNotification, NotificationBaseURL: s.NotificationBaseURL,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	view, err := s.StaffView(ctx, req, row.ID)
	if err == nil {
		view.EmailStatus = emailStatus
	}
	return view, err
}

func requestFolder(db *gorm.DB, tenantID, clientID, folderID string) (*models.File, error) {
	var folder models.File
	err := db.Where("tenant_id = ? AND client_id = ? AND id = ? AND type = ? AND deleted_at IS NULL AND disk_deleted_at IS NULL", tenantID, clientID, folderID, "folder").First(&folder).Error
	return &folder, err
}

func (s *DocumentRequestService) List(ctx context.Context, req AccessRequest) ([]DocumentRequestView, error) {
	db := s.DB.WithContext(ctx)
	if err := s.staff(ctx, db, req, false); err != nil {
		return nil, err
	}
	var rows []models.DocumentRequest
	if err := db.Where("tenant_id = ? AND client_id = ?", req.TenantID, req.ClientID).Order("created_at DESC, id DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]DocumentRequestView, 0, len(rows))
	for _, row := range rows {
		views = append(views, DocumentRequestView{ID: row.ID, Title: row.Title, ExpiresAt: row.ExpiresAt, Revoked: row.RevokedAt != nil})
	}
	return views, nil
}

func (s *DocumentRequestService) StaffView(ctx context.Context, req AccessRequest, id string) (*DocumentRequestView, error) {
	db := s.DB.WithContext(ctx)
	if err := s.staff(ctx, db, req, false); err != nil {
		return nil, err
	}
	var row models.DocumentRequest
	if err := db.Where("tenant_id = ? AND client_id = ? AND id = ?", req.TenantID, req.ClientID, id).First(&row).Error; err != nil {
		return nil, err
	}
	return s.view(db, row, true)
}

func (s *DocumentRequestService) publicRow(db *gorm.DB, tenantID, token string, lock bool) (models.DocumentRequest, error) {
	var row models.DocumentRequest
	id, _, ok := strings.Cut(token, ".")
	if !ok || tenantID == "" || len(id) != 26 || len(token) != 70 || s.SigningKey == "" ||
		!hmac.Equal([]byte(token), []byte(s.token(tenantID, id))) {
		return row, gorm.ErrRecordNotFound
	}
	q := db
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.Where("tenant_id = ? AND id = ? AND revoked_at IS NULL", tenantID, id).First(&row).Error; err != nil {
		return row, err
	}
	if !row.ExpiresAt.After(time.Now()) {
		return row, gorm.ErrRecordNotFound
	}
	var client models.Client
	if err := db.Select("id").Where("tenant_id = ? AND id = ?", tenantID, row.ClientID).First(&client).Error; err != nil {
		return row, err
	}
	_, err := requestFolder(db, tenantID, row.ClientID, row.FolderID)
	return row, err
}

func (s *DocumentRequestService) PublicView(ctx context.Context, tenantID, token string) (*DocumentRequestView, error) {
	db := s.DB.WithContext(ctx)
	row, err := s.publicRow(db, tenantID, token, false)
	if err != nil {
		return nil, err
	}
	return s.view(db, row, false)
}

func liveRequestFiles(db *gorm.DB, row models.DocumentRequest) *gorm.DB {
	return db.Table("document_request_files AS drf").
		Joins("JOIN files f ON f.id = drf.file_id AND f.tenant_id = ? AND f.client_id = ?", row.TenantID, row.ClientID).
		Where("f.deleted_at IS NULL AND f.disk_deleted_at IS NULL AND NOT f.storage_missing")
}

func (s *DocumentRequestService) view(db *gorm.DB, row models.DocumentRequest, staff bool) (*DocumentRequestView, error) {
	var items []models.DocumentRequestItem
	if err := db.Where("request_id = ?", row.ID).Order("position").Find(&items).Error; err != nil {
		return nil, err
	}
	view := &DocumentRequestView{Title: row.Title, ExpiresAt: row.ExpiresAt, Items: make([]DocumentRequestItemView, 0, len(items))}
	if staff {
		view.ID, view.Revoked = row.ID, row.RevokedAt != nil
		folder, err := requestFolder(db, row.TenantID, row.ClientID, row.FolderID)
		if err == nil {
			view.FolderID, view.FolderName = folder.ID, folder.Filename
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if !view.Revoked && row.ExpiresAt.After(time.Now()) {
			view.Link = "/document-request#token=" + s.token(row.TenantID, row.ID)
		}
	}
	for _, item := range items {
		out := DocumentRequestItemView{ID: item.ID, Name: item.Name, Instructions: item.Instructions,
			Status: item.Status, Reason: item.Reason, Round: item.Round, Files: []DocumentRequestFile{}}
		query := liveRequestFiles(db, row).Where("drf.item_id = ?", item.ID)
		if staff {
			query = query.Select("drf.file_id AS id, drf.name, drf.round")
			if err := db.Model(&models.DocumentRequestReview{}).Where("item_id = ?", item.ID).
				Select("round, status, reason, submitted_at, created_at AS reviewed_at").Order("round DESC").Scan(&out.History).Error; err != nil {
				return nil, err
			}
		} else {
			query = query.Where("drf.round = ?", item.Round).Select("drf.name, drf.round")
		}
		if err := query.Order("drf.created_at, drf.file_id").Scan(&out.Files).Error; err != nil {
			return nil, err
		}
		view.Items = append(view.Items, out)
	}
	return view, nil
}

func findRequestItem(db *gorm.DB, requestID, itemID string) (models.DocumentRequestItem, error) {
	var item models.DocumentRequestItem
	err := db.Where("request_id = ? AND id = ?", requestID, itemID).First(&item).Error
	return item, err
}

func (s *DocumentRequestService) Upload(ctx context.Context, tenantID, token, itemID string, round int, name string, src io.Reader, size int64) error {
	if round < 1 || size <= 0 || len(name) > 220 || LogicalName(name) != nil {
		return ErrDocumentRequestInput
	}
	db := s.DB.WithContext(ctx)
	row, err := s.publicRow(db, tenantID, token, false)
	if err != nil {
		return err
	}
	item, err := findRequestItem(db, row.ID, itemID)
	if err != nil {
		return err
	}
	if item.Round != round || (item.Status != requestNeeded && item.Status != requestReplacement) {
		return ErrDocumentRequestConflict
	}
	folder, err := requestFolder(db, tenantID, row.ClientID, row.FolderID)
	if err != nil {
		return err
	}
	ext := filepath.Ext(name)
	filename := strings.TrimSuffix(name, ext) + "-" + ids.NewULID() + ext
	file := &models.File{
		TenantID: tenantID, ClientID: row.ClientID, FolderID: &folder.ID,
		Filename: filename, Path: filepath.Join(folder.Path, filename), Type: "file",
	}
	return uploadFile(ctx, s.DB, s.Store.Tenant(tenantID), file, src, size, func(tx *gorm.DB) error {
		current, err := s.publicRow(tx, tenantID, token, true)
		if err != nil {
			return err
		}
		currentItem, err := findRequestItem(tx, current.ID, itemID)
		if err != nil {
			return err
		}
		if currentItem.Round != round || (currentItem.Status != requestNeeded && currentItem.Status != requestReplacement) {
			return ErrDocumentRequestConflict
		}
		var count int64
		if err := liveRequestFiles(tx, current).Where("drf.item_id = ? AND drf.round = ?", itemID, round).Count(&count).Error; err != nil {
			return err
		}
		if count >= requestMaxFiles {
			return ErrDocumentRequestFiles
		}
		return tx.Create(&models.DocumentRequestFile{FileID: file.ID, ItemID: itemID, Round: round, Name: name}).Error
	})
}

func (s *DocumentRequestService) Submit(ctx context.Context, tenantID, token, itemID string, round int) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := s.publicRow(tx, tenantID, token, true)
		if err != nil {
			return err
		}
		item, err := findRequestItem(tx, row.ID, itemID)
		if err != nil {
			return err
		}
		if item.Round != round {
			return ErrDocumentRequestConflict
		}
		if item.Status == requestSubmitted {
			return nil
		}
		if item.Status != requestNeeded && item.Status != requestReplacement {
			return ErrDocumentRequestConflict
		}
		var count int64
		if err := liveRequestFiles(tx, row).Where("drf.item_id = ? AND drf.round = ?", itemID, round).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrDocumentRequestConflict
		}
		return tx.Model(&item).Updates(map[string]any{"status": requestSubmitted, "reason": "", "submitted_at": time.Now().UTC()}).Error
	})
}

func (s *DocumentRequestService) Review(ctx context.Context, req AccessRequest, id, itemID string, round int, status, reason string) error {
	reason = strings.TrimSpace(reason)
	if (status != requestAccepted && status != requestReplacement) || (status == requestReplacement && reason == "") || utf8.RuneCountInString(reason) > 1000 {
		return ErrDocumentRequestInput
	}
	if status == requestAccepted {
		reason = ""
	}
	return s.mutateStaff(ctx, req, id, func(tx *gorm.DB, row models.DocumentRequest) error {
		item, err := findRequestItem(tx, row.ID, itemID)
		if err != nil {
			return err
		}
		if item.Round != round || item.Status != requestSubmitted || item.SubmittedAt == nil {
			return ErrDocumentRequestConflict
		}
		if status == requestAccepted {
			var count, total int64
			if err := liveRequestFiles(tx, row).Where("drf.item_id = ? AND drf.round = ?", itemID, round).Count(&count).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.DocumentRequestFile{}).Where("item_id = ? AND round = ?", itemID, round).Count(&total).Error; err != nil {
				return err
			}
			if count == 0 || count != total {
				return ErrDocumentRequestConflict
			}
		}
		if err := tx.Create(&models.DocumentRequestReview{ItemID: itemID, Round: round, Status: status, Reason: reason, ReviewerID: req.Actor.UserID, SubmittedAt: *item.SubmittedAt}).Error; err != nil {
			return err
		}
		nextRound := round
		if status == requestReplacement {
			nextRound++
		}
		return tx.Model(&item).Updates(map[string]any{"status": status, "reason": reason, "round": nextRound, "submitted_at": nil}).Error
	})
}

func (s *DocumentRequestService) Revoke(ctx context.Context, req AccessRequest, id string) error {
	return s.mutateStaff(ctx, req, id, func(tx *gorm.DB, row models.DocumentRequest) error {
		if row.RevokedAt != nil {
			return nil
		}
		return tx.Model(&row).Update("revoked_at", time.Now().UTC()).Error
	})
}

func (s *DocumentRequestService) mutateStaff(ctx context.Context, req AccessRequest, id string, mutate func(*gorm.DB, models.DocumentRequest) error) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockTenantAccess(tx, req.TenantID); err != nil {
			return err
		}
		if err := s.staff(ctx, tx, req, false); err != nil {
			return err
		}
		var row models.DocumentRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_id = ? AND client_id = ? AND id = ?", req.TenantID, req.ClientID, id).First(&row).Error; err != nil {
			return err
		}
		return mutate(tx, row)
	})
}
