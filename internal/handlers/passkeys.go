package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/gofiber/fiber/v3"
	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	passkeyChallengeLogin        = "login"
	passkeyChallengeHostedLogin  = "hosted_login"
	passkeyChallengeRegistration = "registration"
	passkeyChallengeTTL          = 5 * time.Minute
)

var errPasskeyChallengeInvalid = errors.New("invalid or expired passkey challenge")
var errHostedPasskeyVerificationFailed = errors.New("hosted passkey verification failed")

type passkeyUser struct {
	user        models.User
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte {
	return []byte(u.user.ID)
}

func (u passkeyUser) WebAuthnName() string {
	return u.user.Email
}

func (u passkeyUser) WebAuthnDisplayName() string {
	if strings.TrimSpace(u.user.Name) != "" {
		return u.user.Name
	}
	return u.user.Email
}

func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

type passkeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func (h *AuthHandler) ListPasskeysHandler(c fiber.Ctx) error {
	user, err := h.currentPasskeyUser(c)
	if err != nil {
		return passkeyUserError(c, err)
	}

	credentials, err := h.listPasskeyCredentialRecords(c, user.TenantID, user.ID)
	if err != nil {
		utils.Logger(c).Error("failed listing passkeys", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load passkeys"})
	}
	return c.JSON(passkeyResponses(credentials))
}

func (h *AuthHandler) BeginPasskeyRegistrationHandler(c fiber.Ctx) error {
	user, err := h.currentPasskeyUser(c)
	if err != nil {
		return passkeyUserError(c, err)
	}

	passkeyUser, _, err := h.loadPasskeyUser(c, user)
	if err != nil {
		utils.Logger(c).Error("failed loading passkey credentials", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to begin passkey setup"})
	}
	webAuthn, err := h.webAuthnForRequest(c, user.TenantID)
	if err != nil {
		utils.Logger(c).Error("failed configuring WebAuthn", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Passkeys are not configured"})
	}

	creation, session, err := webAuthn.BeginRegistration(
		passkeyUser,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired}),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(webauthn.Credentials(passkeyUser.credentials).CredentialDescriptors()),
	)
	if err != nil {
		utils.Logger(c).Error("failed creating passkey registration options", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to begin passkey setup"})
	}

	challengeID, err := h.storePasskeyChallenge(c, user.TenantID, user.ID, passkeyChallengeRegistration, *session)
	if err != nil {
		utils.Logger(c).Error("failed storing passkey registration challenge", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to begin passkey setup"})
	}

	return c.JSON(fiber.Map{"challenge_id": challengeID, "public_key": creation.Response})
}

func (h *AuthHandler) FinishPasskeyRegistrationHandler(c fiber.Ctx) error {
	user, err := h.currentPasskeyUser(c)
	if err != nil {
		return passkeyUserError(c, err)
	}

	var req struct {
		ChallengeID string          `json:"challenge_id"`
		Credential  json.RawMessage `json:"credential"`
		Name        string          `json:"name"`
	}
	if err := c.Bind().Body(&req); err != nil || strings.TrimSpace(req.ChallengeID) == "" || len(req.Credential) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid passkey response"})
	}

	passkeyUser, _, err := h.loadPasskeyUser(c, user)
	if err != nil {
		utils.Logger(c).Error("failed loading passkey credentials", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not verify passkey"})
	}
	session, err := h.consumePasskeyChallenge(c, user.TenantID, user.ID, passkeyChallengeRegistration, req.ChallengeID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid or expired passkey setup"})
	}
	parsedCredential, err := protocol.ParseCredentialCreationResponseBytes(req.Credential)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Could not verify passkey"})
	}
	webAuthn, err := h.webAuthnForRPID(c, user.TenantID, session.RelyingPartyID, []string{tenantWebAuthnOrigin(c, h.resetBaseURL)})
	if err != nil {
		utils.Logger(c).Error("failed configuring WebAuthn", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Passkeys are not configured"})
	}
	credential, err := webAuthn.CreateCredential(passkeyUser, session, parsedCredential)
	if err != nil {
		utils.Logger(c).Info("passkey registration verification failed", zap.Error(err), zap.String("user_id", user.ID))
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Could not verify passkey"})
	}

	encodedCredential, err := json.Marshal(credential)
	if err != nil {
		utils.Logger(c).Error("failed serializing passkey", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save passkey"})
	}
	name, err := h.passkeyName(c, user.TenantID, user.ID, req.Name)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	stored := models.PasskeyCredential{
		TenantID:         user.TenantID,
		UserID:           user.ID,
		CredentialID:     base64.RawURLEncoding.EncodeToString(credential.ID),
		CredentialIDHash: passkeyCredentialHash(credential.ID),
		CredentialData:   encodedCredential,
		RPID:             session.RelyingPartyID,
		Name:             name,
		CreatedAt:        time.Now(),
	}
	if err := h.db.WithContext(c.Context()).Create(&stored).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "This passkey is already registered"})
		}
		utils.Logger(c).Error("failed saving passkey", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not save passkey"})
	}

	return c.Status(fiber.StatusCreated).JSON(passkeyResponseFromModel(stored))
}

func (h *AuthHandler) BeginPasskeyLoginHandler(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind().Body(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email is required"})
	}

	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND email = ?", tenantID, strings.TrimSpace(req.Email)).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.JSON(fiber.Map{"passkey": false})
		}
		utils.Logger(c).Error("failed looking up passkey sign-in user", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to begin sign in"})
	}

	baseURL := tenantBaseURLFromCtx(c, h.resetBaseURL)
	rpID, err := webAuthnRPID(baseURL)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Passkeys are not configured"})
	}
	passkeyUser, records, err := h.loadPasskeyUserForRPID(c, user, rpID)
	if err != nil {
		utils.Logger(c).Error("failed loading passkey credentials", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to begin sign in"})
	}
	if len(records) == 0 {
		return c.JSON(fiber.Map{"passkey": false})
	}
	webAuthn, err := h.webAuthnForBaseURL(c, tenantID, baseURL)
	if err != nil {
		utils.Logger(c).Error("failed configuring WebAuthn", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Passkeys are not configured"})
	}
	assertion, session, err := webAuthn.BeginLogin(passkeyUser, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		utils.Logger(c).Error("failed creating passkey login options", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to begin sign in"})
	}
	challengeID, err := h.storePasskeyChallenge(c, tenantID, user.ID, passkeyChallengeLogin, *session)
	if err != nil {
		utils.Logger(c).Error("failed storing passkey login challenge", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to begin sign in"})
	}
	return c.JSON(fiber.Map{"passkey": true, "challenge_id": challengeID, "public_key": assertion.Response})
}

func (h *AuthHandler) FinishPasskeyLoginHandler(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	var req struct {
		Email       string          `json:"email"`
		ChallengeID string          `json:"challenge_id"`
		Credential  json.RawMessage `json:"credential"`
	}
	if err := c.Bind().Body(&req); err != nil || strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.ChallengeID) == "" || len(req.Credential) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid passkey response"})
	}

	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND email = ?", tenantID, strings.TrimSpace(req.Email)).First(&user).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Passkey sign in failed"})
	}
	session, err := h.consumePasskeyChallenge(c, tenantID, user.ID, passkeyChallengeLogin, req.ChallengeID)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Passkey sign in failed"})
	}
	parsedCredential, err := protocol.ParseCredentialRequestResponseBytes(req.Credential)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Passkey sign in failed"})
	}
	passkeyUser, _, err := h.loadPasskeyUserForRPID(c, user, session.RelyingPartyID)
	if err != nil {
		utils.Logger(c).Error("failed loading passkey credentials", zap.Error(err))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Passkey sign in failed"})
	}
	webAuthn, err := h.webAuthnForRPID(c, tenantID, session.RelyingPartyID, []string{tenantWebAuthnOrigin(c, h.resetBaseURL)})
	if err != nil {
		utils.Logger(c).Error("failed configuring WebAuthn", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Passkeys are not configured"})
	}
	credential, err := webAuthn.ValidateLogin(passkeyUser, session, parsedCredential)
	if err != nil {
		utils.Logger(c).Info("passkey sign-in verification failed", zap.Error(err), zap.String("user_id", user.ID))
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Passkey sign in failed"})
	}
	encodedCredential, err := json.Marshal(credential)
	if err != nil {
		utils.Logger(c).Error("failed serializing passkey", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Passkey sign in failed"})
	}
	now := time.Now()
	credentialIDHash := passkeyCredentialHash(credential.ID)
	update := h.db.WithContext(c.Context()).Model(&models.PasskeyCredential{}).
		Where("tenant_id = ? AND user_id = ? AND credential_id_hash = ? AND (rp_id = ? OR rp_id = '')", tenantID, user.ID, credentialIDHash, session.RelyingPartyID).
		Updates(map[string]interface{}{"credential_data": encodedCredential, "last_used_at": now, "rp_id": session.RelyingPartyID})
	if update.Error != nil || update.RowsAffected != 1 {
		utils.Logger(c).Error("failed updating passkey after sign-in", zap.Error(update.Error))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Passkey sign in failed"})
	}

	token, err := auth.GenerateJWT(user.ID, user.Role, user.TenantID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate token"})
	}
	setAuthSessionCookie(c, token)
	return c.JSON(fiber.Map{"authenticated": true})
}

// BeginHostedPasskeyLogin starts a ceremony from the configured SaaS origin while
// retaining the tenant hostname as the credential's RP ID.
func (h *AuthHandler) BeginHostedPasskeyLogin(c fiber.Ctx, hostedLogin *services.HostedLoginService, email string) (fiber.Map, error) {
	if h.hostedPasskeyOrigin == "" {
		return fiber.Map{"passkey": false}, nil
	}
	identity, user, err := h.hostedPasskeyIdentity(c, hostedLogin, email)
	if err != nil {
		return nil, err
	}
	_, records, err := h.loadPasskeyUser(c, user)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return fiber.Map{"passkey": false}, nil
	}
	legacyRPID, err := webAuthnRPID(identity.RedirectURL)
	if err != nil {
		return nil, err
	}
	groups := passkeyRecordsByRPID(records, legacyRPID)
	rpIDs := make([]string, 0, len(groups))
	for rpID := range groups {
		rpIDs = append(rpIDs, rpID)
	}
	sort.Strings(rpIDs)
	options := make([]fiber.Map, 0, len(rpIDs))
	for _, rpID := range rpIDs {
		passkeyUser, err := passkeyUserFromRecords(user, groups[rpID])
		if err != nil {
			return nil, err
		}
		webAuthn, err := h.webAuthnForRPID(c, identity.TenantID, rpID, []string{h.hostedPasskeyOrigin})
		if err != nil {
			return nil, err
		}
		assertion, session, err := webAuthn.BeginLogin(passkeyUser, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return nil, err
		}
		challengeID, err := h.storePasskeyChallenge(c, identity.TenantID, user.ID, passkeyChallengeHostedLogin, *session)
		if err != nil {
			return nil, err
		}
		options = append(options, fiber.Map{"challenge_id": challengeID, "public_key": assertion.Response})
	}
	return fiber.Map{"passkey": len(options) > 0, "options": options}, nil
}

func (h *AuthHandler) FinishHostedPasskeyLogin(c fiber.Ctx, hostedLogin *services.HostedLoginService, email, challengeID string, credentialJSON json.RawMessage) (*services.HostedLoginResult, error) {
	if h.hostedPasskeyOrigin == "" {
		return nil, errors.New("hosted passkey login is unavailable")
	}
	identity, user, err := h.hostedPasskeyIdentity(c, hostedLogin, email)
	if err != nil {
		return nil, err
	}
	session, err := h.consumePasskeyChallenge(c, identity.TenantID, user.ID, passkeyChallengeHostedLogin, challengeID)
	if err != nil {
		return nil, errHostedPasskeyVerificationFailed
	}
	passkeyUser, _, err := h.loadPasskeyUserForRPID(c, user, session.RelyingPartyID)
	if err != nil {
		return nil, err
	}
	parsedCredential, err := protocol.ParseCredentialRequestResponseBytes(credentialJSON)
	if err != nil {
		return nil, errHostedPasskeyVerificationFailed
	}
	webAuthn, err := h.webAuthnForRPID(c, identity.TenantID, session.RelyingPartyID, []string{h.hostedPasskeyOrigin})
	if err != nil {
		return nil, err
	}
	credential, err := webAuthn.ValidateLogin(passkeyUser, session, parsedCredential)
	if err != nil {
		return nil, errHostedPasskeyVerificationFailed
	}
	encodedCredential, err := json.Marshal(credential)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	update := h.db.WithContext(c.Context()).Model(&models.PasskeyCredential{}).
		Where("tenant_id = ? AND user_id = ? AND credential_id_hash = ? AND (rp_id = ? OR rp_id = '')", identity.TenantID, user.ID, passkeyCredentialHash(credential.ID), session.RelyingPartyID).
		Updates(map[string]interface{}{"credential_data": encodedCredential, "last_used_at": now, "rp_id": session.RelyingPartyID})
	if update.Error != nil || update.RowsAffected != 1 {
		if update.Error != nil {
			return nil, update.Error
		}
		return nil, errors.New("passkey credential was not found")
	}
	return hostedLogin.IssueHandoff(*identity)
}

func (h *AuthHandler) RenamePasskeyHandler(c fiber.Ctx) error {
	user, err := h.currentPasskeyUser(c)
	if err != nil {
		return passkeyUserError(c, err)
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Passkey name must be between 1 and 100 characters"})
	}
	var credential models.PasskeyCredential
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND user_id = ? AND id = ?", user.TenantID, user.ID, c.Params("id")).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Passkey not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not update passkey"})
	}
	if err := h.db.WithContext(c.Context()).Model(&credential).Update("name", name).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not update passkey"})
	}
	credential.Name = name
	return c.JSON(passkeyResponseFromModel(credential))
}

func (h *AuthHandler) DeletePasskeyHandler(c fiber.Ctx) error {
	user, err := h.currentPasskeyUser(c)
	if err != nil {
		return passkeyUserError(c, err)
	}
	result := h.db.WithContext(c.Context()).Where("tenant_id = ? AND user_id = ? AND id = ?", user.TenantID, user.ID, c.Params("id")).Delete(&models.PasskeyCredential{})
	if result.Error != nil {
		utils.Logger(c).Error("failed deleting passkey", zap.Error(result.Error))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not delete passkey"})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Passkey not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) DismissPasskeyPromptHandler(c fiber.Ctx) error {
	user, err := h.currentPasskeyUser(c)
	if err != nil {
		return passkeyUserError(c, err)
	}
	if err := h.db.WithContext(c.Context()).Model(&models.User{}).
		Where("tenant_id = ? AND id = ?", user.TenantID, user.ID).
		Update("passkey_prompt_dismissed", true).Error; err != nil {
		utils.Logger(c).Error("failed dismissing passkey prompt", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not update passkey preference"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *AuthHandler) currentPasskeyUser(c fiber.Ctx) (models.User, error) {
	role, _ := c.Locals("role").(string)
	userID, _ := c.Locals("user_id").(string)
	if role == "link" || strings.TrimSpace(userID) == "" {
		return models.User{}, fiber.NewError(fiber.StatusForbidden, "Passkeys are unavailable for secure links")
	}
	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", tenantIDFromCtx(c), userID).First(&user).Error; err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (h *AuthHandler) hostedPasskeyIdentity(c fiber.Ctx, hostedLogin *services.HostedLoginService, email string) (*services.HostedLoginIdentity, models.User, error) {
	if hostedLogin == nil {
		return nil, models.User{}, errors.New("hosted login is unavailable")
	}
	identity, err := hostedLogin.Resolve(email)
	if err != nil {
		return nil, models.User{}, err
	}
	var user models.User
	if err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND id = ?", identity.TenantID, identity.UserID).First(&user).Error; err != nil {
		return nil, models.User{}, err
	}
	return identity, user, nil
}

func passkeyUserError(c fiber.Ctx, err error) error {
	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return c.Status(fiberErr.Code).JSON(fiber.Map{"error": fiberErr.Message})
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	utils.Logger(c).Error("failed loading passkey user", zap.Error(err))
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load account"})
}

func (h *AuthHandler) loadPasskeyUser(c fiber.Ctx, user models.User) (passkeyUser, []models.PasskeyCredential, error) {
	records, err := h.listPasskeyCredentialRecords(c, user.TenantID, user.ID)
	if err != nil {
		return passkeyUser{}, nil, err
	}
	loadedUser, err := passkeyUserFromRecords(user, records)
	if err != nil {
		return passkeyUser{}, nil, err
	}
	return loadedUser, records, nil
}

func (h *AuthHandler) loadPasskeyUserForRPID(c fiber.Ctx, user models.User, rpID string) (passkeyUser, []models.PasskeyCredential, error) {
	records, err := h.listPasskeyCredentialRecords(c, user.TenantID, user.ID)
	if err != nil {
		return passkeyUser{}, nil, err
	}
	selected := make([]models.PasskeyCredential, 0, len(records))
	for _, record := range records {
		if record.RPID == "" || record.RPID == rpID {
			selected = append(selected, record)
		}
	}
	loadedUser, err := passkeyUserFromRecords(user, selected)
	if err != nil {
		return passkeyUser{}, nil, err
	}
	return loadedUser, selected, nil
}

func passkeyUserFromRecords(user models.User, records []models.PasskeyCredential) (passkeyUser, error) {
	credentials := make([]webauthn.Credential, 0, len(records))
	for _, record := range records {
		var credential webauthn.Credential
		if err := json.Unmarshal(record.CredentialData, &credential); err != nil {
			return passkeyUser{}, fmt.Errorf("decode credential %s: %w", record.ID, err)
		}
		credentials = append(credentials, credential)
	}
	return passkeyUser{user: user, credentials: credentials}, nil
}

func passkeyRecordsByRPID(records []models.PasskeyCredential, legacyRPID string) map[string][]models.PasskeyCredential {
	groups := make(map[string][]models.PasskeyCredential)
	for _, record := range records {
		rpID := record.RPID
		if rpID == "" {
			rpID = legacyRPID
		}
		groups[rpID] = append(groups[rpID], record)
	}
	return groups
}

func (h *AuthHandler) listPasskeyCredentialRecords(c fiber.Ctx, tenantID, userID string) ([]models.PasskeyCredential, error) {
	credentials := make([]models.PasskeyCredential, 0)
	err := h.db.WithContext(c.Context()).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Order("created_at ASC").Find(&credentials).Error
	return credentials, err
}

func (h *AuthHandler) webAuthnForRequest(c fiber.Ctx, tenantID string) (*webauthn.WebAuthn, error) {
	return h.webAuthnForBaseURL(c, tenantID, tenantBaseURLFromCtx(c, h.resetBaseURL))
}

func (h *AuthHandler) webAuthnForBaseURL(c fiber.Ctx, tenantID, publicBaseURL string) (*webauthn.WebAuthn, error) {
	baseURL, err := url.Parse(publicBaseURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Hostname() == "" {
		return nil, errors.New("invalid public base URL")
	}
	origin := baseURL.Scheme + "://" + baseURL.Host
	return h.webAuthnForRPID(c, tenantID, baseURL.Hostname(), []string{origin})
}

func (h *AuthHandler) webAuthnForRPID(c fiber.Ctx, tenantID, rpID string, origins []string) (*webauthn.WebAuthn, error) {
	rpID = strings.TrimSpace(strings.ToLower(rpID))
	if rpID == "" {
		return nil, errors.New("invalid relying party ID")
	}
	allowedOrigins := make([]string, 0, len(origins))
	for _, origin := range origins {
		origin = normalizeWebAuthnOrigin(origin)
		if origin != "" {
			allowedOrigins = append(allowedOrigins, origin)
		}
	}
	if len(allowedOrigins) == 0 {
		return nil, errors.New("invalid relying party origin")
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: h.siteTitleForTenant(c, tenantID),
		RPID:          rpID,
		RPOrigins:     allowedOrigins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: passkeyChallengeTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: passkeyChallengeTTL},
		},
	})
}

func webAuthnRPID(publicBaseURL string) (string, error) {
	parsed, err := url.Parse(publicBaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return "", errors.New("invalid public base URL")
	}
	return strings.ToLower(parsed.Hostname()), nil
}

func tenantWebAuthnOrigin(c fiber.Ctx, fallback string) string {
	baseURL, err := url.Parse(tenantBaseURLFromCtx(c, fallback))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return ""
	}
	return baseURL.Scheme + "://" + baseURL.Host
}

func (h *AuthHandler) RelatedOriginsHandler(c fiber.Ctx) error {
	tenantID := tenantIDFromCtx(c)
	if h.hostedPasskeyOrigin == "" || tenantID == "" || !h.isRegisteredTenantHostname(c, tenantID) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	return c.JSON(fiber.Map{"origins": []string{h.hostedPasskeyOrigin}})
}

func (h *AuthHandler) isRegisteredTenantHostname(c fiber.Ctx, tenantID string) bool {
	hostname := strings.ToLower(strings.TrimSpace(c.Hostname()))
	if hostname == "" {
		return false
	}
	var count int64
	if err := h.db.WithContext(c.Context()).Model(&models.TenantDomain{}).
		Where("tenant_id = ? AND LOWER(domain) = ?", tenantID, hostname).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func normalizeWebAuthnOrigin(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (h *AuthHandler) storePasskeyChallenge(c fiber.Ctx, tenantID, userID, purpose string, session webauthn.SessionData) (string, error) {
	payload, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	now := time.Now()
	expiresAt := session.Expires
	if expiresAt.IsZero() {
		expiresAt = now.Add(passkeyChallengeTTL)
	}
	challenge := models.WebAuthnChallenge{
		TenantID:    tenantID,
		UserID:      userID,
		Purpose:     purpose,
		RPID:        session.RelyingPartyID,
		SessionData: payload,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
	}
	if err := h.db.WithContext(c.Context()).Where("expires_at < ? OR used_at IS NOT NULL", now).Delete(&models.WebAuthnChallenge{}).Error; err != nil {
		utils.Logger(c).Warn("failed cleaning up passkey challenges", zap.Error(err))
	}
	if err := h.db.WithContext(c.Context()).Create(&challenge).Error; err != nil {
		return "", err
	}
	return challenge.ID, nil
}

func (h *AuthHandler) consumePasskeyChallenge(c fiber.Ctx, tenantID, userID, purpose, challengeID string) (webauthn.SessionData, error) {
	now := time.Now()
	var challenge models.WebAuthnChallenge
	if err := h.db.WithContext(c.Context()).
		Where("id = ? AND tenant_id = ? AND user_id = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?", challengeID, tenantID, userID, purpose, now).
		First(&challenge).Error; err != nil {
		return webauthn.SessionData{}, errPasskeyChallengeInvalid
	}
	result := h.db.WithContext(c.Context()).Model(&models.WebAuthnChallenge{}).
		Where("id = ? AND used_at IS NULL AND expires_at > ?", challenge.ID, now).
		Update("used_at", now)
	if result.Error != nil || result.RowsAffected != 1 {
		return webauthn.SessionData{}, errPasskeyChallengeInvalid
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(challenge.SessionData, &session); err != nil {
		return webauthn.SessionData{}, errPasskeyChallengeInvalid
	}
	if challenge.RPID != "" && challenge.RPID != session.RelyingPartyID {
		return webauthn.SessionData{}, errPasskeyChallengeInvalid
	}
	return session, nil
}

func (h *AuthHandler) passkeyName(c fiber.Ctx, tenantID, userID, requested string) (string, error) {
	name := strings.TrimSpace(requested)
	if len(name) > 100 {
		return "", errors.New("passkey name must be 100 characters or fewer")
	}
	if name != "" {
		return name, nil
	}
	var count int64
	if err := h.db.WithContext(c.Context()).Model(&models.PasskeyCredential{}).Where("tenant_id = ? AND user_id = ?", tenantID, userID).Count(&count).Error; err != nil {
		return "", err
	}
	if count == 0 {
		return "Passkey", nil
	}
	return fmt.Sprintf("Passkey %d", count+1), nil
}

func passkeyResponses(credentials []models.PasskeyCredential) []passkeyResponse {
	response := make([]passkeyResponse, 0, len(credentials))
	for _, credential := range credentials {
		response = append(response, passkeyResponseFromModel(credential))
	}
	return response
}

func passkeyResponseFromModel(credential models.PasskeyCredential) passkeyResponse {
	return passkeyResponse{
		ID:         credential.ID,
		Name:       credential.Name,
		CreatedAt:  credential.CreatedAt,
		LastUsedAt: credential.LastUsedAt,
	}
}

func passkeyCredentialHash(credentialID []byte) string {
	digest := sha256.Sum256(credentialID)
	return fmt.Sprintf("%x", digest[:])
}

func (h *AuthHandler) shouldOfferPasskeyEnrollment(c fiber.Ctx, user models.User) bool {
	if user.PasskeyPromptDismissed || user.Role == "link" {
		return false
	}
	var count int64
	if err := h.db.WithContext(c.Context()).Model(&models.PasskeyCredential{}).
		Where("tenant_id = ? AND user_id = ?", user.TenantID, user.ID).
		Count(&count).Error; err != nil {
		utils.Logger(c).Warn("failed checking passkey enrollment status", zap.Error(err))
		return false
	}
	return count == 0
}
