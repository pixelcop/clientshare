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
	Title           string                     `json:"title"`
	Description     string                     `json:"description"`
	FolderName      string                     `json:"folder_name"`
	EmailAll        bool                       `json:"email_all"`
	RecipientEmails []string                   `json:"recipient_emails"`
	Items           []DocumentRequestItemInput `json:"items"`
}

type DocumentRequestItemInput struct {
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
}

type DocumentRequestView struct {
	ID          string                      `json:"id,omitempty"`
	Title       string                      `json:"title"`
	Description string                      `json:"description,omitempty"`
	Updates     []DocumentRequestUpdateView `json:"updates,omitempty"`
	ExpiresAt   time.Time                   `json:"expires_at"`
	Revoked     bool                        `json:"revoked,omitempty"`
	Link        string                      `json:"link,omitempty"`
	FolderID    string                      `json:"folder_id,omitempty"`
	FolderName  string                      `json:"folder_name,omitempty"`
	EmailStatus string                      `json:"email_status,omitempty"`
	EmailCount  int                         `json:"email_count,omitempty"`
	Items       []DocumentRequestItemView   `json:"items"`
}

type DocumentRequestUpdateView struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
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
	ID          string     `json:"id,omitempty"` // unsubmitted drafts have no recipient viewer link
	Name        string     `json:"name"`
	Round       int        `json:"round"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	Comments    int        `json:"comment_count,omitempty"`
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

func documentRequestRecipients(ctx context.Context, db *gorm.DB, req AccessRequest, input DocumentRequestInput) ([]models.User, error) {
	if !input.EmailAll && len(input.RecipientEmails) == 0 {
		return nil, nil
	}
	query := db.Where("tenant_id = ? AND role IN ?", req.TenantID, []string{"client", "customer"})
	associations := db.Table("(?) AS associations", recipientAssociations(db, req.TenantID)).
		Select("user_id").Where("client_id = ?", req.ClientID)
	query = query.Where("id IN (?)", associations)
	requested := make(map[string]bool)
	if !input.EmailAll {
		addresses := make([]string, 0, len(input.RecipientEmails))
		for _, value := range input.RecipientEmails {
			address := strings.ToLower(email.CommentNotificationAddress(value))
			if address == "" {
				return nil, ErrDocumentRequestRecipient
			}
			if !requested[address] {
				requested[address] = true
				addresses = append(addresses, address)
			}
		}
		query = query.Where("LOWER(email) IN ?", addresses)
	}
	var users []models.User
	if err := query.Order("id").Find(&users).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	recipients := users[:0]
	matched := make(map[string]bool)
	for _, user := range users {
		allowed, err := (&AccessService{DB: db}).DocumentRequestRecipient(ctx, req.ClientID, user, now)
		if err != nil {
			return nil, err
		}
		if allowed {
			matched[strings.ToLower(email.CommentNotificationAddress(user.Email))] = true
			recipients = append(recipients, user)
		}
	}
	if !input.EmailAll && len(matched) != len(requested) {
		return nil, ErrDocumentRequestRecipient
	}
	return recipients, nil
}

func (s *DocumentRequestService) Create(ctx context.Context, req AccessRequest, input DocumentRequestInput) (*DocumentRequestView, error) {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if s.SigningKey == "" || input.Title == "" || utf8.RuneCountInString(input.Title) > 160 || utf8.RuneCountInString(input.Description) > 4000 {
		return nil, ErrDocumentRequestInput
	}
	name := strings.TrimSpace(input.FolderName)
	if name == "" {
		name = input.Title
	}
	if len(name) > 255 || LogicalName(name) != nil || strings.ContainsAny(name, "\r\n") {
		return nil, ErrDocumentRequestFolder
	}
	if len(input.RecipientEmails) > 100 || (input.EmailAll && len(input.RecipientEmails) != 0) {
		return nil, ErrDocumentRequestInput
	}
	emailStatus := ""
	emailCount := 0
	row := models.DocumentRequest{
		ID: ids.NewULID(), TenantID: req.TenantID, ClientID: req.ClientID, Title: input.Title, Description: input.Description,
		CreatedBy: req.Actor.UserID, ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
	}
	items, err := newRequestItems(row.ID, input.Items)
	if err != nil {
		return nil, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := LockTenantAccess(tx, req.TenantID); err != nil {
			return err
		}
		if err := s.staff(ctx, tx, req, true); err != nil {
			return err
		}
		recipients, err := documentRequestRecipients(ctx, tx, req, input)
		if err != nil {
			return err
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
		staff, err := requestActivityAudience(ctx, tx, row, false)
		if err != nil {
			return err
		}
		if err := recordDocumentRequestActivity(ctx, tx, row, models.FeedEventTypeRequestCreated, models.FeedEventSourceTypeDocumentRequestStaff, row.ID, &row.CreatedBy, staff, row.CreatedAt); err != nil {
			return err
		}
		if len(recipients) == 0 {
			if input.EmailAll {
				emailStatus = "no_recipients"
			}
			return nil
		}
		users := make([]string, 0, len(recipients))
		for _, recipient := range recipients {
			users = append(users, recipient.ID)
		}
		if err := recordDocumentRequestActivity(ctx, tx, row, models.FeedEventTypeRequestCreated, models.FeedEventSourceTypeDocumentRequest, row.ID, &row.CreatedBy, users, row.CreatedAt); err != nil {
			return err
		}
		emailStatus, emailCount, err = s.queueRequestEmails(tx, row, recipients, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	view, err := s.StaffView(ctx, req, row.ID)
	if err == nil {
		view.EmailStatus = emailStatus
		view.EmailCount = emailCount
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
	view := &DocumentRequestView{Title: row.Title, Description: row.Description, ExpiresAt: row.ExpiresAt, Items: make([]DocumentRequestItemView, 0, len(items))}
	if err := db.Model(&models.DocumentRequestUpdate{}).Where("request_id = ? AND description <> ''", row.ID).
		Order("created_at DESC, id DESC").Find(&view.Updates).Error; err != nil {
		return nil, err
	}
	if staff {
		view.ID, view.Revoked = row.ID, row.RevokedAt != nil
		folder, err := requestFolder(db, row.TenantID, row.ClientID, row.FolderID)
		if err == nil {
			view.FolderID, view.FolderName = folder.ID, folder.Filename
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if view.FolderID != "" && !view.Revoked && row.ExpiresAt.After(time.Now()) {
			view.Link = "/document-request#token=" + s.token(row.TenantID, row.ID)
		}
	}
	for _, item := range items {
		out := DocumentRequestItemView{ID: item.ID, Name: item.Name, Instructions: item.Instructions,
			Status: item.Status, Reason: item.Reason, Round: item.Round, Files: []DocumentRequestFile{}}
		query := requestFilesWithSubmission(db, row).Where("drf.item_id = ?", item.ID)
		comments := db.Model(&models.FileComment{}).Select("COUNT(*)").
			Where("tenant_id = ? AND file_id = drf.file_id AND deleted_at IS NULL", row.TenantID)
		fileID := "drf.file_id"
		var history []DocumentRequestReview
		if err := db.Model(&models.DocumentRequestReview{}).Where("item_id = ?", item.ID).
			Select("round, status, reason, submitted_at, created_at AS reviewed_at").Order("round DESC").Scan(&history).Error; err != nil {
			return nil, err
		}
		if staff {
			out.History = history
		} else {
			comments = comments.Where("visibility = ? AND "+requestFileSubmittedAt+" IS NOT NULL", CommentShared)
			fileID = "CASE WHEN " + requestFileSubmittedAt + " IS NOT NULL THEN drf.file_id ELSE '' END"
			query = query.Where("drf.round = ? OR "+requestFileSubmittedAt+" IS NOT NULL", item.Round)
		}
		query = query.Select(fileID+" AS id, drf.name, drf.round, (?) AS comments", comments)
		if err := query.Order("drf.created_at, drf.file_id").Scan(&out.Files).Error; err != nil {
			return nil, err
		}
		dates := map[int]*time.Time{item.Round: item.SubmittedAt}
		for _, review := range history {
			dates[review.Round] = &review.SubmittedAt
		}
		for i := range out.Files {
			out.Files[i].SubmittedAt = dates[out.Files[i].Round]
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
		now := time.Now().UTC()
		if err := tx.Model(&item).Updates(map[string]any{"status": requestSubmitted, "reason": "", "submitted_at": now}).Error; err != nil {
			return err
		}
		return requestItemActivity(ctx, tx, row, item.ID, round, models.FeedEventTypeRequestSubmitted, nil, now)
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
		now := time.Now().UTC()
		if err := tx.Create(&models.DocumentRequestReview{ItemID: itemID, Round: round, Status: status, Reason: reason, ReviewerID: req.Actor.UserID, SubmittedAt: item.SubmittedAt.UTC(), CreatedAt: now}).Error; err != nil {
			return err
		}
		nextRound := round
		eventType := models.FeedEventTypeRequestAccepted
		if status == requestReplacement {
			nextRound++
			eventType = models.FeedEventTypeRequestReplacement
		}
		if err := tx.Model(&item).Updates(map[string]any{"status": status, "reason": reason, "round": nextRound, "submitted_at": nil}).Error; err != nil {
			return err
		}
		return requestItemActivity(ctx, tx, row, item.ID, round, eventType, &req.Actor.UserID, now)
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
