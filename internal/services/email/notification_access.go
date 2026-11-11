package email

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/pixelcop/clientshare/internal/access"
	"github.com/pixelcop/clientshare/internal/models"
	"gorm.io/gorm"
)

type NotificationRecipient struct {
	Email  string
	UserID *string
	LinkID *string
}

// Item grantees are not automatically subscribed to client-wide updates. Keep
// the existing member/staff notification direction, but bound manager scope
// and require current read permission for every eventual message item.
func UploadNotificationRecipients(ctx context.Context, db *gorm.DB, tenantID, clientID, actorRole string, now time.Time) ([]NotificationRecipient, error) {
	var users []models.User
	var links []models.SecureLink
	var candidateRoles []string
	switch actorRole {
	case "admin", "manager":
		candidateRoles = []string{"client", "customer"}
		if err := db.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND email <> '' AND revoked_at IS NULL AND access_type IN ?", tenantID, clientID, []string{"read", "view", "readwrite"}).Find(&links).Error; err != nil {
			return nil, err
		}
	case "client", "customer", "link":
		candidateRoles = []string{"admin", "manager"}
	default:
		return nil, nil
	}
	ids, err := (&access.AccessService{DB: db}).WorkspaceReaderIDs(ctx, tenantID, clientID, candidateRoles)
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		if err := db.WithContext(ctx).Where("tenant_id = ? AND id IN ?", tenantID, ids).Find(&users).Error; err != nil {
			return nil, err
		}
	}
	result := []NotificationRecipient{}
	for _, user := range users {
		if strings.TrimSpace(user.Email) != "" {
			id := user.ID
			result = append(result, NotificationRecipient{Email: user.Email, UserID: &id})
		}
	}
	for _, link := range links {
		if link.ExpiresAt.After(now) {
			id := link.ID
			result = append(result, NotificationRecipient{Email: link.Email, LinkID: &id})
		}
	}
	return result, nil
}

// BuildUploadNotification binds one message to one current identity/contact and
// rebuilds its content from authorized active resources. It is used at enqueue
// and dispatch; stored rendered text is never evidence of continued access.
func BuildUploadNotification(ctx context.Context, db *gorm.DB, tenantID, clientID, recipient string, userID, linkID *string, ids []string, baseURL, productName string, now time.Time) (Email, bool, error) {
	var empty Email
	if (userID == nil) == (linkID == nil) || tenantID == "" || clientID == "" {
		return empty, false, nil
	}
	address, err := mail.ParseAddress(recipient)
	if err != nil || len(splitRecipients(recipient)) != 1 {
		return empty, false, nil
	}
	actor := access.AccessActor{}
	if userID != nil {
		var user models.User
		if err := db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, *userID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return empty, false, nil
			}
			return empty, false, err
		}
		if !strings.EqualFold(strings.TrimSpace(user.Email), address.Address) {
			return empty, false, nil
		}
		actor.UserID = user.ID
	} else {
		var link models.SecureLink
		if err := db.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND id = ?", tenantID, clientID, *linkID).First(&link).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return empty, false, nil
			}
			return empty, false, err
		}
		if !strings.EqualFold(strings.TrimSpace(link.Email), address.Address) {
			return empty, false, nil
		}
		actor.LinkID = link.ID
	}
	var client models.Client
	if err := db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, clientID).First(&client).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return empty, false, nil
		}
		return empty, false, err
	}
	unique := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	resolver := &access.AccessService{DB: db}
	req := access.AccessRequest{TenantID: tenantID, ClientID: clientID, Actor: actor, Operation: access.AccessRead, Now: now}
	files := []models.File{}
	allowedIDs := []string{}
	for start := 0; start < len(unique); start += access.MaxItems {
		batch := unique[start:min(start+access.MaxItems, len(unique))]
		decisions, err := resolver.ResolveItems(ctx, req, batch)
		if err != nil {
			return empty, false, err
		}
		var rows []models.File
		if err := db.WithContext(ctx).Where("tenant_id = ? AND client_id = ? AND id IN ? AND type = 'file' AND deleted_at IS NULL AND disk_deleted_at IS NULL", tenantID, clientID, batch).Find(&rows).Error; err != nil {
			return empty, false, err
		}
		byID := map[string]models.File{}
		for _, file := range rows {
			byID[file.ID] = file
		}
		for _, id := range batch {
			file, ok := byID[id]
			if ok && decisions[id].Allowed {
				// Email templates need only the authorized item's own name/ID.
				file.Path, file.FolderID, file.UserID = file.Filename, nil, nil
				files = append(files, file)
				allowedIDs = append(allowedIDs, id)
			}
		}
	}
	if len(files) == 0 {
		return empty, false, nil
	}
	clientAccess, err := resolver.Resolve(ctx, req)
	if err != nil {
		return empty, false, err
	}
	label := "Shared files"
	if clientAccess.Allowed {
		label = client.Name
	}
	subject, body, htmlBody := RenderUploadNotification(label, files, baseURL, productName)
	return Email{TenantID: tenantID, To: address.Address, HideRecipients: true, Subject: subject, Body: body, HTMLBody: htmlBody, ClientID: &clientID, FileIDsCSV: strings.Join(allowedIDs, ","), RecipientUserID: userID, RecipientLinkID: linkID, NotificationBaseURL: baseURL, NotificationProductName: productName}, true, nil
}

func (q *DBEmailQueue) revalidateUploadNotification(item *models.QueuedEmail) (bool, error) {
	if item.ClientID == nil {
		return false, nil
	}
	email, valid, err := BuildUploadNotification(context.Background(), q.DB, item.TenantID, *item.ClientID, item.Recipient, item.RecipientUserID, item.RecipientLinkID, strings.Split(item.FileIDsCSV, ","), item.NotificationBaseURL, item.NotificationProductName, q.now())
	if err != nil || !valid {
		return valid, err
	}
	item.Subject, item.Body, item.HTMLBody, item.FileIDsCSV = email.Subject, email.Body, email.HTMLBody, email.FileIDsCSV
	result := q.DB.Model(&models.QueuedEmail{}).Where("tenant_id = ? AND id = ? AND sent_at IS NULL AND canceled_at IS NULL", item.TenantID, item.ID).Updates(map[string]any{"subject": item.Subject, "body": item.Body, "html_body": item.HTMLBody, "file_ids_csv": item.FileIDsCSV})
	return result.RowsAffected == 1, result.Error
}
