package email

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/access"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/links"
	"gorm.io/gorm"
)

const DocumentRequestNotification = "document_request"

func DocumentRequestRecipient(ctx context.Context, db *gorm.DB, clientID string, user models.User, now time.Time) (bool, error) {
	if (user.Role != "client" && user.Role != "customer") || CommentNotificationAddress(user.Email) == "" {
		return false, nil
	}
	decision, err := (&access.AccessService{DB: db}).Resolve(ctx, access.AccessRequest{
		TenantID: user.TenantID, ClientID: clientID, Actor: access.AccessActor{UserID: user.ID},
		Operation: access.AccessContext, Now: now,
	})
	return decision.Allowed && (decision.Capabilities.CanRead || decision.Capabilities.CanUpload), err
}

func (q *DBEmailQueue) revalidateDocumentRequest(item *models.QueuedEmail) (bool, error) {
	if item.EventType != DocumentRequestNotification || item.ClientID == nil || item.RecipientUserID == nil || q.DocumentRequestSigningKey == "" {
		return false, nil
	}
	ctx := context.Background()
	db := q.DB.WithContext(ctx)
	var request models.DocumentRequest
	if err := db.Where("tenant_id = ? AND client_id = ? AND id = ? AND revoked_at IS NULL",
		item.TenantID, *item.ClientID, *item.DocumentRequestID).First(&request).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if !request.ExpiresAt.After(q.now()) {
		return false, nil
	}
	var folders int64
	if err := db.Model(&models.File{}).Where("tenant_id = ? AND client_id = ? AND id = ? AND type = 'folder' AND deleted_at IS NULL AND disk_deleted_at IS NULL",
		request.TenantID, request.ClientID, request.FolderID).Count(&folders).Error; err != nil || folders != 1 {
		return false, err
	}
	var user models.User
	if err := db.Where("tenant_id = ? AND id = ?", item.TenantID, *item.RecipientUserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if !strings.EqualFold(user.Email, item.Recipient) {
		return false, nil
	}
	allowed, err := DocumentRequestRecipient(ctx, db, request.ClientID, user, q.now())
	if err != nil || !allowed {
		return false, err
	}
	base, err := url.Parse(item.NotificationBaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil {
		return false, nil
	}
	base.Path, base.RawPath, base.RawQuery = "/document-request", "", ""
	base.Fragment = "token=" + links.DocumentRequestToken(q.DocumentRequestSigningKey, request.TenantID, request.ID)
	item.Subject = "New document request"
	item.Body = "You have a new document request. Open the checklist to see what’s needed and submit files:\n\n" + base.String()
	item.HTMLBody = ""
	return true, nil
}
