package services

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

// A standards-compliant none-attestation response with an ES256 public key.
func signupAttestation(t *testing.T, session webauthn.SessionData, origin string, verified bool) json.RawMessage {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	publicKey, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: key.X.FillBytes(make([]byte, 32)), -3: key.Y.FillBytes(make([]byte, 32))})
	require.NoError(t, err)
	rpHash := sha256.Sum256([]byte(session.RelyingPartyID))
	authData := append([]byte{}, rpHash[:]...)
	flags := byte(0x41) // user presence and attested credential data
	if verified {
		flags |= 0x04
	}
	authData = append(authData, flags, 0, 0, 0, 0)
	authData = append(authData, make([]byte, 16)...)
	credentialID := []byte("signup-credential")
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(credentialID)))
	authData = append(authData, credentialID...)
	authData = append(authData, publicKey...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	require.NoError(t, err)
	clientData, err := json.Marshal(map[string]any{"type": "webauthn.create", "challenge": session.Challenge, "origin": origin})
	require.NoError(t, err)
	encoded := base64.RawURLEncoding.EncodeToString
	response, err := json.Marshal(map[string]any{"id": encoded(credentialID), "rawId": encoded(credentialID), "type": "public-key", "response": map[string]any{
		"clientDataJSON": encoded(clientData), "attestationObject": encoded(attestation), "transports": []string{"internal"},
	}, "clientExtensionResults": map[string]any{}})
	require.NoError(t, err)
	return response
}

func TestPasskeySignupVerificationAndProvisioning(t *testing.T) {
	db := openTenantProvisioningTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.PasskeySignup{}, &models.PasskeyCredential{}))
	svc := NewPasskeySignupService(db, "https://saas.test")
	ctx := context.Background()
	options, err := svc.Begin(ctx, "Owner@firm.test", "Owner", "firm")
	require.NoError(t, err)
	token := options["signup_token"].(string)
	var record models.PasskeySignup
	require.NoError(t, db.First(&record).Error)
	require.NotEqual(t, token, record.TokenHash)
	var session webauthn.SessionData
	require.NoError(t, json.Unmarshal(record.SessionData, &session))
	require.Equal(t, "saas.test", session.RelyingPartyID)
	require.ErrorIs(t, svc.Reserve(ctx, token, record.Email, "firm"), ErrPasskeySignupInvalid)
	require.ErrorIs(t, svc.Verify(ctx, token, signupAttestation(t, session, "https://evil.test", true)), ErrPasskeySignupInvalid)
	require.ErrorIs(t, svc.Verify(ctx, token, signupAttestation(t, session, "https://saas.test", false)), ErrPasskeySignupInvalid)
	wrongChallenge := session
	wrongChallenge.Challenge = "different-challenge"
	require.ErrorIs(t, svc.Verify(ctx, token, signupAttestation(t, wrongChallenge, "https://saas.test", true)), ErrPasskeySignupInvalid)
	response := signupAttestation(t, session, "https://saas.test", true)
	require.NoError(t, svc.Verify(ctx, token, response))
	require.ErrorIs(t, svc.Verify(ctx, token, response), ErrPasskeySignupInvalid)
	require.ErrorIs(t, svc.Reserve(ctx, token, "other@firm.test", "firm"), ErrPasskeySignupInvalid)
	require.ErrorIs(t, svc.Reserve(ctx, token, record.Email, "other-firm"), ErrPasskeySignupInvalid)
	require.NoError(t, svc.Reserve(ctx, token, record.Email, "firm"))
	// Accepted signups remain provisionable across delayed email/checkout callbacks.
	require.NoError(t, db.Model(&record).Update("expires_at", time.Now().Add(-time.Hour)).Error)
	require.NoError(t, svc.Reserve(ctx, token, record.Email, "firm"))
	require.NoError(t, db.Create(&models.Tenant{ID: "tenant-firm", Slug: "firm", Name: "Firm"}).Error)
	require.NoError(t, db.Create(&models.Tenant{ID: "tenant-other", Slug: "other", Name: "Other"}).Error)
	provisioning := NewTenantProvisioningService(db, "secret", 0, nil, "https://portal.test")
	input := CreateAdminUserInput{Email: record.Email, Name: "Owner", PasskeySignupToken: token}
	_, err = provisioning.CreateAdminUser("tenant-other", input)
	require.ErrorIs(t, err, ErrPasskeySignupInvalid)
	user, err := provisioning.CreateAdminUser("tenant-firm", input)
	require.NoError(t, err)
	require.Equal(t, record.UserID, user.ID)
	require.Empty(t, user.PasswordHash)
	var credential models.PasskeyCredential
	require.NoError(t, db.First(&credential, "user_id = ?", user.ID).Error)
	require.Equal(t, "saas.test", credential.RPID)
	require.Equal(t, "tenant-firm", credential.TenantID)
	retry, err := provisioning.CreateAdminUser("tenant-firm", input)
	require.NoError(t, err)
	require.Equal(t, user.ID, retry.ID)
	var count int64
	require.NoError(t, db.Model(&models.PasskeyCredential{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestPasskeySignupExpiredChallenge(t *testing.T) {
	db := openTenantProvisioningTestDB(t)
	require.NoError(t, db.AutoMigrate(&models.PasskeySignup{}))
	svc := NewPasskeySignupService(db, "https://saas.test")
	options, err := svc.Begin(context.Background(), "owner@firm.test", "Owner", "firm")
	require.NoError(t, err)
	require.NoError(t, db.Model(&models.PasskeySignup{}).Where("1 = 1").Update("expires_at", time.Now().Add(-time.Minute)).Error)
	require.ErrorIs(t, svc.Verify(context.Background(), options["signup_token"].(string), json.RawMessage(`{}`)), ErrPasskeySignupInvalid)
}
