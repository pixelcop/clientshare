package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/models"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ClientImportService struct {
	DB             *gorm.DB
	Storage        storage.Storage
	EmailQueue     emailpkg.EmailQueue
	TenantID       string
	BaseURL        string
	TokenSecret    string
	InviteTTL      time.Duration
	TenantSettings *TenantSettingsService

	clientService *ClientService
}

type ClientImportOptions struct {
	InvitedBy    string
	InviteSendAt *time.Time
}

type ClientImportResult struct {
	RowsRead        int
	CreatedClients  int
	ExistingClients int
	CreatedUsers    int
	UpdatedUsers    int
	InvitedUsers    int
	InviteEmailIDs  []string
}

type clientImportCSVData struct {
	clientNames []string
	userEmails  []string
	rowsRead    int
	clients     map[string]struct{}
	users       map[string]map[string]struct{}
}

func (s *ClientImportService) ImportCSV(reader io.Reader, options ClientImportOptions) (*ClientImportResult, error) {
	if s.DB == nil {
		return nil, errors.New("database not configured")
	}
	tenantID, err := requireTenantID(s.TenantID)
	if err != nil {
		return nil, err
	}

	importData, err := parseClientImportCSV(reader)
	if err != nil {
		return nil, err
	}

	result := &ClientImportResult{RowsRead: importData.rowsRead}
	clientIDsByName := make(map[string]string, len(importData.clientNames))
	for _, clientName := range importData.clientNames {
		client, created, err := s.findOrCreateClient(tenantID, clientName)
		if err != nil {
			return nil, err
		}
		clientIDsByName[clientName] = client.ID
		if created {
			result.CreatedClients++
		} else {
			result.ExistingClients++
		}
	}

	for _, email := range importData.userEmails {
		clientIDs := make([]string, 0, len(importData.users[email]))
		for clientName := range importData.users[email] {
			clientIDs = append(clientIDs, clientIDsByName[clientName])
		}
		clientIDs = normalizeClientIDs(clientIDs)
		if len(clientIDs) == 0 {
			continue
		}

		created, updated, inviteEmailID, err := s.upsertImportedUser(tenantID, email, clientIDs, options)
		if err != nil {
			return nil, err
		}
		if created {
			result.CreatedUsers++
		}
		if updated {
			result.UpdatedUsers++
		}
		if inviteEmailID != "" {
			result.InvitedUsers++
			result.InviteEmailIDs = append(result.InviteEmailIDs, inviteEmailID)
		}
	}

	sort.Strings(result.InviteEmailIDs)
	return result, nil
}

func (s *ClientImportService) clientSvc() *ClientService {
	if s.clientService == nil {
		s.clientService = &ClientService{DB: s.DB, Storage: s.Storage}
	}
	return s.clientService
}

func (s *ClientImportService) findOrCreateClient(tenantID, name string) (*models.Client, bool, error) {
	trimmedName := strings.TrimSpace(name)
	var existing models.Client
	err := s.DB.Where("tenant_id = ? AND name = ?", tenantID, trimmedName).First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	client, createErr := s.clientSvc().CreateClient(context.Background(), tenantID, trimmedName)
	if createErr != nil {
		return nil, false, createErr
	}
	return client, true, nil
}

func (s *ClientImportService) upsertImportedUser(tenantID, email string, clientIDs []string, options ClientImportOptions) (bool, bool, string, error) {
	var existing models.User
	err := s.DB.Where("tenant_id = ? AND LOWER(email) = ?", tenantID, strings.ToLower(email)).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, "", err
	}

	now := time.Now()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if s.EmailQueue == nil {
			return false, false, "", errors.New("email queue is required for new user invites")
		}
		if strings.TrimSpace(s.TokenSecret) == "" {
			return false, false, "", errors.New("invite token secret is required for new user invites")
		}
		inviteRawToken, tokenErr := generateInviteToken()
		if tokenErr != nil {
			return false, false, "", tokenErr
		}
		expiresAt := now.Add(s.inviteTTL())
		user := models.User{
			TenantID:     tenantID,
			Email:        email,
			PasswordHash: "",
			Role:         "client",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		inviteRecord := models.InviteToken{}
		if err := s.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
			for _, clientID := range clientIDs {
				if err := addClientAssignment(tx, tenantID, user.ID, clientID); err != nil {
					return err
				}
			}
			inviteRecord = models.InviteToken{
				TenantID:  tenantID,
				UserID:    user.ID,
				Email:     user.Email,
				TokenHash: hashInviteToken(s.TokenSecret, inviteRawToken),
				ExpiresAt: expiresAt,
				InvitedBy: strings.TrimSpace(options.InvitedBy),
				CreatedAt: now,
			}
			return tx.Create(&inviteRecord).Error
		}); err != nil {
			return false, false, "", err
		}
		user.ClientIDs = clientIDs
		inviteEmailID, err := s.queueUserInviteEmail(user, inviteRawToken, inviteRecord.ExpiresAt, options.InviteSendAt)
		if err != nil {
			return false, false, "", err
		}
		return true, false, inviteEmailID, nil
	}

	existingClientIDs, err := s.listClientIDsForUser(tenantID, existing.ID)
	if err != nil {
		return false, false, "", err
	}
	mergedClientIDs := normalizeClientIDs(append(existingClientIDs, clientIDs...))
	updated := len(mergedClientIDs) != len(existingClientIDs)
	if updated {
		if err := s.DB.Transaction(func(tx *gorm.DB) error {
			for _, clientID := range clientIDs {
				if err := addClientAssignment(tx, tenantID, existing.ID, clientID); err != nil {
					return err
				}
			}
			existing.UpdatedAt = now
			return tx.Save(&existing).Error
		}); err != nil {
			return false, false, "", err
		}
	}

	return false, updated, "", nil
}

func (s *ClientImportService) queueUserInviteEmail(user models.User, rawToken string, expiresAt time.Time, sendAt *time.Time) (string, error) {
	baseURL, siteTitle, inviteWelcomeText := s.inviteEmailSettings(user.TenantID)
	inviteURL := buildInviteAcceptURL(baseURL, rawToken)
	subject, body, htmlBody := emailpkg.RenderUserInviteEmail(user.Name, inviteURL, siteTitle, inviteWelcomeText, expiresAt)
	queued, err := s.EmailQueue.Queue(emailpkg.Email{
		TenantID: user.TenantID,
		To:       user.Email,
		Subject:  subject,
		Body:     body,
		HTMLBody: htmlBody,
	}, sendAt)
	if err != nil {
		return "", err
	}
	return queued.ID, nil
}

func (s *ClientImportService) inviteEmailSettings(tenantID string) (string, string, string) {
	baseURL := strings.TrimSpace(s.BaseURL)
	siteTitle := DefaultSiteTitle
	inviteWelcomeText := ""
	if s.TenantSettings == nil {
		return baseURL, siteTitle, inviteWelcomeText
	}
	settings, err := s.TenantSettings.Get(context.Background(), tenantID)
	if err != nil {
		zap.L().Warn("failed loading tenant settings for client import", zap.Error(err), zap.String("tenant_id", tenantID))
		return baseURL, siteTitle, inviteWelcomeText
	}
	if strings.TrimSpace(settings.PublicBaseURL) != "" {
		baseURL = settings.PublicBaseURL
	}
	return baseURL, SiteTitleOrDefault(settings), strings.TrimSpace(settings.InviteWelcomeText)
}

func (s *ClientImportService) inviteTTL() time.Duration {
	if s.InviteTTL > 0 {
		return s.InviteTTL
	}
	return 72 * time.Hour
}

func parseClientImportCSV(reader io.Reader) (*clientImportCSVData, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true

	data := &clientImportCSVData{
		clients: make(map[string]struct{}),
		users:   make(map[string]map[string]struct{}),
	}

	rowNumber := 0
	for {
		record, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read csv: %w", err)
		}
		rowNumber++

		if len(record) == 1 && strings.TrimSpace(record[0]) == "" {
			continue
		}
		if len(record) != 2 {
			return nil, fmt.Errorf("row %d must contain exactly 2 columns", rowNumber)
		}

		clientName := strings.TrimSpace(record[0])
		email := strings.TrimSpace(record[1])
		if rowNumber == 1 && isClientImportHeaderRow(clientName, email) {
			continue
		}
		if clientName == "" {
			zap.L().Warn("skipping row with empty client name", zap.Int("row", rowNumber))
			continue
		}

		data.rowsRead++
		if _, ok := data.clients[clientName]; !ok {
			data.clients[clientName] = struct{}{}
			data.clientNames = append(data.clientNames, clientName)
		}

		if email == "" {
			continue
		}
		resolvedEmails, err := parseClientImportEmails(email)
		if err != nil {
			return nil, fmt.Errorf("row %d has invalid email", rowNumber)
		}
		for _, resolvedEmail := range resolvedEmails {
			clientNames, ok := data.users[resolvedEmail]
			if !ok {
				clientNames = make(map[string]struct{})
				data.users[resolvedEmail] = clientNames
				data.userEmails = append(data.userEmails, resolvedEmail)
			}
			clientNames[clientName] = struct{}{}
		}
	}

	if data.rowsRead == 0 {
		return nil, errors.New("csv file is empty")
	}
	sort.Strings(data.userEmails)
	return data, nil
}

func isClientImportHeaderRow(clientColumn, emailColumn string) bool {
	normalizedClient := normalizeClientImportHeader(clientColumn)
	normalizedEmail := normalizeClientImportHeader(emailColumn)

	clientHeader := normalizedClient == "clientname" || normalizedClient == "client"
	emailHeader := normalizedEmail == "email" || normalizedEmail == "emailaddress" || normalizedEmail == "emailaddresses"

	return clientHeader && emailHeader
}

func normalizeClientImportHeader(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func parseClientImportEmails(value string) ([]string, error) {
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		return nil, err
	}
	resolved := make([]string, 0, len(parsed))
	seen := make(map[string]struct{}, len(parsed))
	for _, addr := range parsed {
		if addr == nil {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(addr.Address))
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		resolved = append(resolved, email)
	}
	if len(resolved) == 0 {
		return nil, errors.New("no valid email addresses")
	}
	return resolved, nil
}

func normalizeClientIDs(clientIDs []string) []string {
	seen := make(map[string]struct{}, len(clientIDs))
	filtered := make([]string, 0, len(clientIDs))
	for _, raw := range clientIDs {
		clientID := strings.TrimSpace(raw)
		if clientID == "" {
			continue
		}
		if _, ok := seen[clientID]; ok {
			continue
		}
		seen[clientID] = struct{}{}
		filtered = append(filtered, clientID)
	}
	sort.Strings(filtered)
	return filtered
}

func addClientAssignment(tx *gorm.DB, tenantID, userID, clientID string) error {
	rel := models.UserClient{TenantID: strings.TrimSpace(tenantID), UserID: strings.TrimSpace(userID), ClientID: strings.TrimSpace(clientID), CreatedAt: time.Now()}
	if rel.UserID == "" || rel.ClientID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "user_id and client_id are required")
	}
	if rel.TenantID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "tenant_id is required")
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rel).Error
}

func (s *ClientImportService) listClientIDsForUser(tenantID, userID string) ([]string, error) {
	type row struct {
		ClientID string
	}
	rows := make([]row, 0)
	if err := s.DB.Table("user_clients").
		Select("client_id").
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Order("client_id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	clientIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		clientIDs = append(clientIDs, row.ClientID)
	}
	return clientIDs, nil
}

func buildInviteAcceptURL(baseURL, token string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" || strings.TrimSpace(token) == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/accept-invite#token=" + token
}

func generateInviteToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashInviteToken(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
