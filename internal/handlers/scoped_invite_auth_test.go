package handlers_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/auth"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/stretchr/testify/require"
)

func TestIdentityInviteAcceptsOnlyItsItemTransactionally(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "revoked"}[revoked], func(t *testing.T) {
			env := setupOtherHandlersEnv(t)
			file := createFileRecord(t, env, env.rootFolder, "invited-item.txt", "item content")
			user := models.User{TenantID: env.client.TenantID, Email: "new-item@example.test", Role: "client"}
			require.NoError(t, env.db.Create(&user).Error)
			service := &services.SharingService{DB: env.db, TokenSecret: "invite-secret"}
			intent, err := service.CreateInvitation(t.Context(), services.AccessRequest{TenantID: env.client.TenantID, ClientID: env.client.ID, FileID: file.ID, Actor: services.AccessActor{UserID: env.adminUser.ID}}, services.InvitationInput{Email: user.Email, Capabilities: models.AccessCapabilities{CanRead: true}}, "identity-item-invite-key")
			require.NoError(t, err)
			raw := "test-only-identity-token"
			mac := hmac.New(sha256.New, []byte("invite-secret"))
			_, _ = mac.Write([]byte(raw))
			proof := models.InviteToken{TenantID: env.client.TenantID, UserID: user.ID, Email: user.Email, TokenHash: hex.EncodeToString(mac.Sum(nil)), AccessInvitationID: &intent.ID, ExpiresAt: time.Now().Add(time.Hour), InvitedBy: env.adminUser.ID}
			require.NoError(t, env.db.Create(&proof).Error)
			if revoked {
				require.NoError(t, env.db.Model(intent).Update("revoked_at", time.Now()).Error)
			}
			resp, err := env.app.Test(jsonRequest(http.MethodPost, "/api/auth/accept-invite", map[string]any{"token": raw, "password": "recipient-password"}))
			require.NoError(t, err)
			cookies := resp.Cookies()
			var result map[string]any
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
			resp.Body.Close()
			require.NoError(t, env.db.First(&user, "id = ?", user.ID).Error)
			require.NoError(t, env.db.First(&proof, "id = ?", proof.ID).Error)
			var count int64
			require.NoError(t, env.db.Model(&models.UserClient{}).Where("user_id = ?", user.ID).Count(&count).Error)
			require.Zero(t, count, "identity setup for an item never creates membership")
			if revoked {
				require.Equal(t, 400, resp.StatusCode)
				require.Empty(t, user.PasswordHash)
				require.Nil(t, proof.UsedAt, "failed item acceptance rolls back the identity claim")
				return
			}
			require.Equal(t, 200, resp.StatusCode)
			require.NotEmpty(t, user.PasswordHash)
			require.NotNil(t, proof.UsedAt)
			require.Equal(t, "/files/"+file.ID, result["redirect_to"])
			for _, path := range []string{"/api/files/" + file.ID, "/api/folders/" + env.rootFolder.ID} {
				req := jsonRequest(http.MethodGet, path, nil)
				for _, cookie := range cookies {
					req.AddCookie(cookie)
				}
				got, err := env.app.Test(req)
				require.NoError(t, err)
				want := 200
				if strings.HasPrefix(path, "/api/folders/") {
					want = 404
				}
				require.Equal(t, want, got.StatusCode)
				got.Body.Close()
			}
		})
	}
}

func TestStaleIdentityInvitationCannotResetAnAcceptedAccount(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	user := models.User{TenantID: env.client.TenantID, Email: "pending@example.test", Role: "client"}
	require.NoError(t, env.db.Create(&user).Error)
	tokens := []string{"first-identity-token", "second-identity-token"}
	for _, raw := range tokens {
		mac := hmac.New(sha256.New, []byte("invite-secret"))
		_, _ = mac.Write([]byte(raw))
		require.NoError(t, env.db.Create(&models.InviteToken{TenantID: env.client.TenantID, UserID: user.ID, Email: user.Email, TokenHash: hex.EncodeToString(mac.Sum(nil)), ExpiresAt: time.Now().Add(time.Hour)}).Error)
	}
	resp, err := env.app.Test(jsonRequest(http.MethodPost, "/api/auth/accept-invite", map[string]any{"token": tokens[1], "password": "owned-password"}))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	resp, err = env.app.Test(jsonRequest(http.MethodPost, "/api/auth/accept-invite", map[string]any{"token": tokens[0], "password": "attacker-password"}))
	require.NoError(t, err)
	require.Equal(t, 400, resp.StatusCode)
	resp.Body.Close()
	require.NoError(t, env.db.First(&user, "id = ?", user.ID).Error)
	require.True(t, auth.CheckPasswordHash("owned-password", user.PasswordHash))
}
