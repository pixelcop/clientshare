package handlers_test

import (
	"net/http"
	"testing"

	"encoding/json"
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/stretchr/testify/require"
)

func TestEmailQueueHandlers_RouteCoverage(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	queued, err := env.queue.Queue(email.Email{To: "a@test.local", Subject: "hello", Body: "body"}, models.Now())
	if err != nil {
		t.Fatalf("failed queueing email: %v", err)
	}

	unauthListReq := jsonRequest(http.MethodGet, "/api/email/queue", nil)
	unauthListResp, err := env.app.Test(unauthListReq)
	if err != nil {
		t.Fatalf("unauth list request failed: %v", err)
	}
	if unauthListResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthListResp.StatusCode)
	}

	listReq := jsonAuthRequest(http.MethodGet, "/api/email/queue", env.adminToken, nil)
	listResp, err := env.app.Test(listReq)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}

	badIDReq := jsonAuthRequest(http.MethodPost, "/api/email/queue/0/send", env.adminToken, nil)
	badIDResp, err := env.app.Test(badIDReq)
	if err != nil {
		t.Fatalf("bad id send request failed: %v", err)
	}
	if badIDResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", badIDResp.StatusCode)
	}

	notFoundSendReq := jsonAuthRequest(http.MethodPost, "/api/email/queue/01ARZ3NDEKTSV4RRFFQ69G5FAV/send", env.adminToken, nil)
	notFoundSendResp, err := env.app.Test(notFoundSendReq)
	if err != nil {
		t.Fatalf("not found send request failed: %v", err)
	}
	if notFoundSendResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", notFoundSendResp.StatusCode)
	}

	sendReq := jsonAuthRequest(http.MethodPost, "/api/email/queue/"+queued.ID+"/send", env.adminToken, nil)
	sendResp, err := env.app.Test(sendReq)
	if err != nil {
		t.Fatalf("send request failed: %v", err)
	}
	if sendResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", sendResp.StatusCode)
	}

	processReq := jsonAuthRequest(http.MethodPost, "/api/email/queue/process", env.adminToken, nil)
	processResp, err := env.app.Test(processReq)
	if err != nil {
		t.Fatalf("process request failed: %v", err)
	}
	if processResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", processResp.StatusCode)
	}
}

func TestEmailQueueManagerScopeAndTenantBoundProcessing(t *testing.T) {
	env := setupOtherHandlersEnv(t)
	assigned, err := env.queue.Queue(email.Email{TenantID: env.client.TenantID, To: "assigned@example.test", Subject: "Assigned", Body: "visible", ClientID: &env.client.ID, FileIDsCSV: "uploaded-file"}, nil)
	require.NoError(t, err)
	private, err := env.queue.Queue(email.Email{TenantID: env.client.TenantID, To: "private@example.test", Subject: "Private", Body: "private-client-content", ClientID: &env.otherClient.ID, FileIDsCSV: "private-file"}, nil)
	require.NoError(t, err)
	credential, err := env.queue.Queue(email.Email{TenantID: env.client.TenantID, To: "identity@example.test", Subject: "Set password", Body: "private-identity-token", ClientID: &env.client.ID}, nil)
	require.NoError(t, err)
	foreign, err := env.queue.Queue(email.Email{TenantID: "foreign", To: "foreign@example.test", Subject: "Foreign", Body: "foreign-content"}, nil)
	require.NoError(t, err)
	resp, err := env.app.Test(jsonAuthRequest(http.MethodGet, "/api/email/queue", env.managerToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	var result struct {
		Count   int                  `json:"count"`
		Pending []models.QueuedEmail `json:"pending"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	resp.Body.Close()
	require.Equal(t, 1, result.Count)
	require.Equal(t, assigned.ID, result.Pending[0].ID)
	for _, item := range []*models.QueuedEmail{private, credential, foreign} {
		resp, err := env.app.Test(jsonAuthRequest(http.MethodPost, "/api/email/queue/"+item.ID+"/send", env.managerToken, nil))
		require.NoError(t, err)
		require.Equal(t, 404, resp.StatusCode)
		resp.Body.Close()
	}
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPost, "/api/email/queue/process", env.managerToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	require.NotNil(t, assigned.SentAt)
	require.Nil(t, private.SentAt)
	require.Nil(t, credential.SentAt)
	require.Nil(t, foreign.SentAt)
	resp, err = env.app.Test(jsonAuthRequest(http.MethodPost, "/api/email/queue/process", env.adminToken, nil))
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	resp.Body.Close()
	require.NotNil(t, private.SentAt)
	require.NotNil(t, credential.SentAt)
	require.Nil(t, foreign.SentAt)
}
