package handlers_test

import (
	"net/http"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/email"
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
