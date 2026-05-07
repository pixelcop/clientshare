package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/assert"
)

type paginatedUsersResponse struct {
	Pagination models.PaginationResponse `json:"pagination"`
	Items      []models.User             `json:"items"`
}

type generatedUserAccessLinkResponse struct {
	Kind      string    `json:"kind"`
	Link      string    `json:"link"`
	ExpiresAt time.Time `json:"expires_at"`
}

func tokenFromLinkFragment(t *testing.T, rawLink string) string {
	t.Helper()

	parsed, err := url.Parse(rawLink)
	if err != nil {
		t.Fatalf("parse generated link failed: %v", err)
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatalf("parse generated link fragment failed: %v", err)
	}
	token := fragment.Get("token")
	if token == "" {
		t.Fatalf("expected generated link token in fragment, got %q", rawLink)
	}
	return token
}

func TestUsersHandler_CreateUserQueuesInviteEmail(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	req := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
		"email": "new-hire@test.local",
		"role":  "manager",
		"name":  "New Hire",
	})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	if len(env.queue.items) == 0 {
		t.Fatalf("expected invite email to be queued")
	}

	queued := env.queue.items[len(env.queue.items)-1]
	if queued.Recipient != "new-hire@test.local" {
		t.Fatalf("expected queued recipient new-hire@test.local, got %q", queued.Recipient)
	}
	if !strings.Contains(strings.ToLower(queued.Subject), "set up your account") {
		t.Fatalf("expected queued subject to include invite setup copy, got %q", queued.Subject)
	}
	if !strings.Contains(strings.ToLower(queued.Body), "set your password") {
		t.Fatalf("expected queued body to include invite setup copy")
	}
}

func TestUsersHandler_ListUsersPaginatedSearchesAssignedClientNames(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
		"email":      "searchable-user@test.local",
		"role":       "client",
		"name":       "Searchable User",
		"client_ids": []string{env.client.ID},
	})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	listReq := jsonAuthRequest(http.MethodGet, "/api/users?page=1&page_size=1&search="+env.client.Name, env.adminToken, nil)
	listResp, err := env.app.Test(listReq)
	if err != nil {
		t.Fatalf("paginated list request failed: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listed paginatedUsersResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode paginated users failed: %v", err)
	}
	if listed.Pagination.TotalItems < 1 {
		t.Fatalf("expected at least 1 matching user, got %d", listed.Pagination.TotalItems)
	}
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 item on page, got %d", len(listed.Items))
	}
	if listed.Items[0].Email != "searchable-user@test.local" {
		t.Fatalf("expected searchable-user@test.local, got %q", listed.Items[0].Email)
	}
	if len(listed.Items[0].ClientIDs) != 1 || listed.Items[0].ClientIDs[0] != env.client.ID {
		t.Fatalf("expected matching client assignment in paginated result")
	}
	if listed.Items[0].InviteAccepted {
		t.Fatalf("expected invited user to be pending until password is set")
	}
}

func TestUsersHandler_ListUsersPaginatedFiltersByRoleAndInviteStatus(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
		"email": "pending-manager@test.local",
		"role":  "manager",
		"name":  "Pending Manager",
	})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	managerReq := jsonAuthRequest(http.MethodGet, "/api/users?page=1&page_size=10&role=manager&invite_status=pending", env.adminToken, nil)
	managerResp, err := env.app.Test(managerReq)
	if err != nil {
		t.Fatalf("manager filter request failed: %v", err)
	}
	if managerResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", managerResp.StatusCode)
	}

	var managerList paginatedUsersResponse
	if err := json.NewDecoder(managerResp.Body).Decode(&managerList); err != nil {
		t.Fatalf("decode manager users failed: %v", err)
	}
	if managerList.Pagination.TotalItems != 1 {
		t.Fatalf("expected 1 pending manager, got %d", managerList.Pagination.TotalItems)
	}
	if len(managerList.Items) != 1 {
		t.Fatalf("expected 1 manager item, got %d", len(managerList.Items))
	}
	if managerList.Items[0].Email != "pending-manager@test.local" {
		t.Fatalf("expected pending-manager@test.local, got %q", managerList.Items[0].Email)
	}
	if managerList.Items[0].Role != "manager" {
		t.Fatalf("expected manager role, got %q", managerList.Items[0].Role)
	}
	if managerList.Items[0].InviteAccepted {
		t.Fatalf("expected pending manager invite to be unaccepted")
	}

	adminReq := jsonAuthRequest(http.MethodGet, "/api/users?page=1&page_size=10&role=admin&invite_status=accepted", env.adminToken, nil)
	adminResp, err := env.app.Test(adminReq)
	if err != nil {
		t.Fatalf("admin filter request failed: %v", err)
	}
	if adminResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", adminResp.StatusCode)
	}

	var adminList paginatedUsersResponse
	if err := json.NewDecoder(adminResp.Body).Decode(&adminList); err != nil {
		t.Fatalf("decode admin users failed: %v", err)
	}
	if adminList.Pagination.TotalItems < 1 {
		t.Fatalf("expected at least 1 accepted admin, got %d", adminList.Pagination.TotalItems)
	}
	if len(adminList.Items) == 0 {
		t.Fatalf("expected at least 1 admin item")
	}
	for _, user := range adminList.Items {
		if user.Role != "admin" {
			t.Fatalf("expected admin role, got %q", user.Role)
		}
		if !user.InviteAccepted {
			t.Fatalf("expected accepted admin invite status")
		}
	}
}

func TestUsersHandler_ListUsersPaginatedFiltersByClientID(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
		"email":      "scoped-client@test.local",
		"role":       "client",
		"name":       "Scoped Client",
		"client_ids": []string{env.client.ID},
	})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	listReq := jsonAuthRequest(http.MethodGet, "/api/users?page=1&page_size=10&client_id="+env.client.ID+"&invite_status=pending", env.adminToken, nil)
	listResp, err := env.app.Test(listReq)
	if err != nil {
		t.Fatalf("client filter request failed: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listed paginatedUsersResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode filtered users failed: %v", err)
	}
	if listed.Pagination.TotalItems != 1 {
		t.Fatalf("expected 1 pending scoped client user, got %d", listed.Pagination.TotalItems)
	}
	if len(listed.Items) != 1 {
		t.Fatalf("expected 1 filtered user, got %d", len(listed.Items))
	}
	if listed.Items[0].Email != "scoped-client@test.local" {
		t.Fatalf("expected scoped-client@test.local, got %q", listed.Items[0].Email)
	}
	if listed.Items[0].Role != "client" {
		t.Fatalf("expected client role, got %q", listed.Items[0].Role)
	}
	if len(listed.Items[0].ClientIDs) != 1 || listed.Items[0].ClientIDs[0] != env.client.ID {
		t.Fatalf("expected filtered user to remain assigned to client %s", env.client.ID)
	}
	if listed.Items[0].InviteAccepted {
		t.Fatalf("expected scoped client invite to be pending")
	}
	for _, user := range listed.Items {
		for _, clientID := range user.ClientIDs {
			if clientID != env.client.ID {
				t.Fatalf("expected only client %s in filtered results, got %s", env.client.ID, clientID)
			}
		}
	}
}

func TestUsersHandler_ResendInviteReissuesPendingInvite(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
		"email": "resend-me@test.local",
		"role":  "manager",
		"name":  "Resend Me",
	})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	var created models.User
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user failed: %v", err)
	}

	beforeQueue := len(env.queue.items)
	resendReq := jsonAuthRequest(http.MethodPost, "/api/users/"+created.ID+"/resend-invite", env.adminToken, nil)
	resendResp, err := env.app.Test(resendReq)
	if err != nil {
		t.Fatalf("resend invite request failed: %v", err)
	}
	if resendResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resendResp.StatusCode)
	}
	if len(env.queue.items) != beforeQueue+1 {
		t.Fatalf("expected one additional queued invite email, got %d items", len(env.queue.items)-beforeQueue)
	}

	var invites []models.InviteToken
	if err := env.db.Where("user_id = ?", created.ID).Order("created_at ASC").Find(&invites).Error; err != nil {
		t.Fatalf("failed to load invite tokens: %v", err)
	}
	if len(invites) != 2 {
		t.Fatalf("expected 2 invite tokens, got %d", len(invites))
	}
	assert.Nil(t, invites[0].UsedAt, "original invite still valid")
	if invites[1].UsedAt != nil {
		t.Fatalf("expected latest invite to remain unused")
	}
	if invites[1].InvitedBy != env.adminUser.ID {
		t.Fatalf("expected resent invite to record inviting admin, got %q", invites[1].InvitedBy)
	}
	if env.queue.items[len(env.queue.items)-1].Recipient != "resend-me@test.local" {
		t.Fatalf("expected resent invite recipient resend-me@test.local, got %q", env.queue.items[len(env.queue.items)-1].Recipient)
	}
}

func TestUsersHandler_GenerateAccessLinkReturnsInviteForPendingUser(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
		"email": "copy-invite@test.local",
		"role":  "manager",
		"name":  "Copy Invite",
	})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	var created models.User
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user failed: %v", err)
	}

	beforeQueue := len(env.queue.items)
	generateReq := jsonAuthRequest(http.MethodPost, "/api/users/"+created.ID+"/generate-access-link", env.adminToken, nil)
	generateResp, err := env.app.Test(generateReq)
	if err != nil {
		t.Fatalf("generate access link request failed: %v", err)
	}
	if generateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", generateResp.StatusCode)
	}
	if len(env.queue.items) != beforeQueue {
		t.Fatalf("expected no additional queued email, got %d new items", len(env.queue.items)-beforeQueue)
	}

	var body generatedUserAccessLinkResponse
	if err := json.NewDecoder(generateResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode generated access link failed: %v", err)
	}
	if body.Kind != "invite" {
		t.Fatalf("expected invite kind, got %q", body.Kind)
	}
	if !strings.Contains(body.Link, "/accept-invite#token=") {
		t.Fatalf("expected invite link, got %q", body.Link)
	}
	if body.ExpiresAt.IsZero() {
		t.Fatalf("expected invite expiration timestamp")
	}
	_ = tokenFromLinkFragment(t, body.Link)

	var invites []models.InviteToken
	if err := env.db.Where("tenant_id = ? AND user_id = ?", env.adminUser.TenantID, created.ID).Order("created_at ASC").Find(&invites).Error; err != nil {
		t.Fatalf("failed to load invite tokens: %v", err)
	}
	if len(invites) != 2 {
		t.Fatalf("expected 2 invite tokens, got %d", len(invites))
	}
	assert.Nil(t, invites[0].UsedAt, "original invite stil valid")
	if invites[1].UsedAt != nil {
		t.Fatalf("expected generated invite to remain unused")
	}
	if invites[1].InvitedBy != env.adminUser.ID {
		t.Fatalf("expected generated invite to record admin user id, got %q", invites[1].InvitedBy)
	}
	if invites[0].TokenHash == invites[1].TokenHash {
		t.Fatalf("expected a fresh invite token hash")
	}

	var resetCount int64
	if err := env.db.Model(&models.PasswordResetToken{}).Where("tenant_id = ? AND user_id = ?", env.adminUser.TenantID, created.ID).Count(&resetCount).Error; err != nil {
		t.Fatalf("count password reset tokens failed: %v", err)
	}
	if resetCount != 0 {
		t.Fatalf("expected no password reset tokens for pending user, got %d", resetCount)
	}
}

func TestUsersHandler_GenerateAccessLinkReturnsResetForAcceptedUser(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	beforeQueue := len(env.queue.items)
	generateReq := jsonAuthRequest(http.MethodPost, "/api/users/"+env.clientUser.ID+"/generate-access-link", env.adminToken, nil)
	generateResp, err := env.app.Test(generateReq)
	if err != nil {
		t.Fatalf("generate password reset link request failed: %v", err)
	}
	if generateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", generateResp.StatusCode)
	}
	if len(env.queue.items) != beforeQueue {
		t.Fatalf("expected no additional queued email, got %d new items", len(env.queue.items)-beforeQueue)
	}

	var body generatedUserAccessLinkResponse
	if err := json.NewDecoder(generateResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode generated password reset link failed: %v", err)
	}
	if body.Kind != "password_reset" {
		t.Fatalf("expected password_reset kind, got %q", body.Kind)
	}
	if !strings.Contains(body.Link, "/reset-password#token=") {
		t.Fatalf("expected password reset link, got %q", body.Link)
	}
	if body.ExpiresAt.IsZero() {
		t.Fatalf("expected password reset expiration timestamp")
	}
	_ = tokenFromLinkFragment(t, body.Link)

	var resets []models.PasswordResetToken
	if err := env.db.Where("tenant_id = ? AND user_id = ?", env.clientUser.TenantID, env.clientUser.ID).Order("created_at ASC").Find(&resets).Error; err != nil {
		t.Fatalf("failed to load password reset tokens: %v", err)
	}
	if len(resets) != 1 {
		t.Fatalf("expected 1 password reset token, got %d", len(resets))
	}
	if resets[0].UsedAt != nil {
		t.Fatalf("expected generated password reset token to remain unused")
	}

	var inviteCount int64
	if err := env.db.Model(&models.InviteToken{}).Where("tenant_id = ? AND user_id = ?", env.clientUser.TenantID, env.clientUser.ID).Count(&inviteCount).Error; err != nil {
		t.Fatalf("count invite tokens failed: %v", err)
	}
	if inviteCount != 0 {
		t.Fatalf("expected no invite tokens for accepted user, got %d", inviteCount)
	}
}

func TestUsersHandler_GenerateAccessLinkReturnsResetWithCustomExpiryForAcceptedUser(t *testing.T) {
	env := setupOtherHandlersEnvWithOptions(t, handlersTestOptions{passwordResetDuration: "720h"})

	beforeQueue := len(env.queue.items)
	generateReq := jsonAuthRequest(http.MethodPost, "/api/users/"+env.clientUser.ID+"/generate-access-link", env.adminToken, nil)
	generateResp, err := env.app.Test(generateReq)
	if err != nil {
		t.Fatalf("generate password reset link request failed: %v", err)
	}
	if generateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", generateResp.StatusCode)
	}
	if len(env.queue.items) != beforeQueue {
		t.Fatalf("expected no additional queued email, got %d new items", len(env.queue.items)-beforeQueue)
	}

	var body generatedUserAccessLinkResponse
	if err := json.NewDecoder(generateResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode generated password reset link failed: %v", err)
	}
	if body.Kind != "password_reset" {
		t.Fatalf("expected password_reset kind, got %q", body.Kind)
	}
	if !strings.Contains(body.Link, "/reset-password#token=") {
		t.Fatalf("expected password reset link, got %q", body.Link)
	}
	if body.ExpiresAt.IsZero() {
		t.Fatalf("expected password reset expiration timestamp")
	}
	_ = tokenFromLinkFragment(t, body.Link)

	var resets []models.PasswordResetToken
	if err := env.db.Where("tenant_id = ? AND user_id = ?", env.clientUser.TenantID, env.clientUser.ID).Order("created_at ASC").Find(&resets).Error; err != nil {
		t.Fatalf("failed to load password reset tokens: %v", err)
	}
	if len(resets) != 1 {
		t.Fatalf("expected 1 password reset token, got %d", len(resets))
	}
	if resets[0].UsedAt != nil {
		t.Fatalf("expected generated password reset token to remain unused")
	}

	customTTL := 720 * time.Hour
	if got := resets[0].ExpiresAt.Sub(resets[0].CreatedAt); got != customTTL {
		t.Fatalf("expected custom reset ttl %s, got %s", customTTL, got)
	}
	if !body.ExpiresAt.Equal(resets[0].ExpiresAt) {
		t.Fatalf("expected response expires_at %s to match stored expires_at %s", body.ExpiresAt.Format(time.RFC3339), resets[0].ExpiresAt.Format(time.RFC3339))
	}

	var inviteCount int64
	if err := env.db.Model(&models.InviteToken{}).Where("tenant_id = ? AND user_id = ?", env.clientUser.TenantID, env.clientUser.ID).Count(&inviteCount).Error; err != nil {
		t.Fatalf("count invite tokens failed: %v", err)
	}
	if inviteCount != 0 {
		t.Fatalf("expected no invite tokens for accepted user, got %d", inviteCount)
	}
}

func TestUsersHandler_ResendAllInvitesQueuesPendingUsers(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	for _, email := range []string{"bulk-one@test.local", "bulk-two@test.local"} {
		createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
			"email": email,
			"role":  "manager",
			"name":  email,
		})
		createResp, err := env.app.Test(createReq)
		if err != nil {
			t.Fatalf("create user request failed: %v", err)
		}
		if createResp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d", createResp.StatusCode)
		}
	}

	beforeQueue := len(env.queue.items)
	resendReq := jsonAuthRequest(http.MethodPost, "/api/users/resend-invites", env.adminToken, nil)
	resendResp, err := env.app.Test(resendReq)
	if err != nil {
		t.Fatalf("resend all invites request failed: %v", err)
	}
	if resendResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resendResp.StatusCode)
	}

	var body struct {
		Resent int `json:"resent"`
	}
	if err := json.NewDecoder(resendResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode resend all response failed: %v", err)
	}
	if body.Resent != 2 {
		t.Fatalf("expected 2 resent invites, got %d", body.Resent)
	}
	if len(env.queue.items) != beforeQueue+2 {
		t.Fatalf("expected 2 additional queued invite emails, got %d", len(env.queue.items)-beforeQueue)
	}
}

func TestUsersHandler_ManagerCanManageUsers(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.managerToken, map[string]any{
		"email": "managed-by-manager@test.local",
		"role":  "manager",
		"name":  "Managed By Manager",
	})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}

	var created models.User
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created user failed: %v", err)
	}

	listReq := jsonAuthRequest(http.MethodGet, "/api/users?page=1&page_size=10&search=managed-by-manager@test.local", env.managerToken, nil)
	listResp, err := env.app.Test(listReq)
	if err != nil {
		t.Fatalf("list users request failed: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	var listed paginatedUsersResponse
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode listed users failed: %v", err)
	}
	if listed.Pagination.TotalItems < 1 {
		t.Fatalf("expected at least one listed user, got %d", listed.Pagination.TotalItems)
	}

	assignReq := jsonAuthRequest(http.MethodPost, "/api/users/"+created.ID+"/assign-client", env.managerToken, map[string]any{"client_id": env.client.ID})
	assignResp, err := env.app.Test(assignReq)
	if err != nil {
		t.Fatalf("assign client request failed: %v", err)
	}
	if assignResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", assignResp.StatusCode)
	}

	var assigned models.User
	if err := json.NewDecoder(assignResp.Body).Decode(&assigned); err != nil {
		t.Fatalf("decode assigned user failed: %v", err)
	}
	foundClientAssignment := false
	for _, clientID := range assigned.ClientIDs {
		if clientID == env.client.ID {
			foundClientAssignment = true
			break
		}
	}
	if !foundClientAssignment {
		t.Fatalf("expected manager assignment to include client %s", env.client.ID)
	}

	updateReq := jsonAuthRequest(http.MethodPut, "/api/users/"+created.ID, env.managerToken, map[string]any{"name": "Manager Updated"})
	updateResp, err := env.app.Test(updateReq)
	if err != nil {
		t.Fatalf("update user request failed: %v", err)
	}
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateResp.StatusCode)
	}

	var updated models.User
	if err := json.NewDecoder(updateResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated user failed: %v", err)
	}
	if updated.Name != "Manager Updated" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}

	beforeResendQueue := len(env.queue.items)
	resendReq := jsonAuthRequest(http.MethodPost, "/api/users/"+created.ID+"/resend-invite", env.managerToken, nil)
	resendResp, err := env.app.Test(resendReq)
	if err != nil {
		t.Fatalf("resend invite request failed: %v", err)
	}
	if resendResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resendResp.StatusCode)
	}
	if len(env.queue.items) != beforeResendQueue+1 {
		t.Fatalf("expected one additional queued invite email, got %d", len(env.queue.items)-beforeResendQueue)
	}

	secondCreateReq := jsonAuthRequest(http.MethodPost, "/api/users", env.managerToken, map[string]any{
		"email":      "second-managed@test.local",
		"role":       "client",
		"name":       "Second Managed",
		"client_ids": []string{env.client.ID},
	})
	secondCreateResp, err := env.app.Test(secondCreateReq)
	if err != nil {
		t.Fatalf("second create user request failed: %v", err)
	}
	if secondCreateResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", secondCreateResp.StatusCode)
	}

	resendAllReq := jsonAuthRequest(http.MethodPost, "/api/users/resend-invites", env.managerToken, nil)
	resendAllResp, err := env.app.Test(resendAllReq)
	if err != nil {
		t.Fatalf("resend all invites request failed: %v", err)
	}
	if resendAllResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resendAllResp.StatusCode)
	}

	var resendAllBody struct {
		Resent int `json:"resent"`
	}
	if err := json.NewDecoder(resendAllResp.Body).Decode(&resendAllBody); err != nil {
		t.Fatalf("decode resend all response failed: %v", err)
	}
	if resendAllBody.Resent < 2 {
		t.Fatalf("expected at least 2 resent invites, got %d", resendAllBody.Resent)
	}

	deleteReq := jsonAuthRequest(http.MethodDelete, "/api/users/"+created.ID, env.managerToken, nil)
	deleteResp, err := env.app.Test(deleteReq)
	if err != nil {
		t.Fatalf("delete user request failed: %v", err)
	}
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteResp.StatusCode)
	}
}

func TestUsersHandler_ClientCannotManageUsers(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	req := jsonAuthRequest(http.MethodGet, "/api/users", env.clientToken, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("list users request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}
