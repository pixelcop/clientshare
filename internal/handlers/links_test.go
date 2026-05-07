package handlers_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services/links"
)

func TestLinkHandlers_RouteCoverage(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	notFoundReq := jsonRequest(http.MethodGet, "/links/not-found-token", nil)
	notFoundResp, err := env.app.Test(notFoundReq)
	if err != nil {
		t.Fatalf("bootstrap not found request failed: %v", err)
	}
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", notFoundResp.StatusCode)
	}

	publicNotFoundReq := jsonAuthRequest(http.MethodGet, "/api/link/not-found-token", env.adminToken, nil)
	publicNotFoundResp, err := env.app.Test(publicNotFoundReq)
	if err != nil {
		t.Fatalf("view link not found request failed: %v", err)
	}
	if publicNotFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", publicNotFoundResp.StatusCode)
	}

	adminCreateReq := jsonAuthRequest(http.MethodPost, "/api/clients/"+intToString(env.client.ID)+"/links", env.adminToken, map[string]any{"access_type": "view", "expiry_days": 1, "email": "viewer@example.com"})
	adminCreateReq.Host = "tenant.example.test"
	adminCreateResp, err := env.app.Test(adminCreateReq)
	if err != nil {
		t.Fatalf("admin create link request failed: %v", err)
	}
	if adminCreateResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", adminCreateResp.StatusCode)
	}
	var createdLink models.SecureLink
	if err := json.NewDecoder(adminCreateResp.Body).Decode(&createdLink); err != nil {
		t.Fatalf("decode link failed: %v", err)
	}
	if createdLink.Email != "viewer@example.com" {
		t.Fatalf("expected link email to persist, got %q", createdLink.Email)
	}
	if createdLink.LinkURL != handlersTestPublicBaseURL+"/links/"+createdLink.Token {
		t.Fatalf("expected absolute link_url, got %q", createdLink.LinkURL)
	}
	if len(env.queue.items) == 0 {
		t.Fatalf("expected secure link email to be queued")
	}

	listReq := jsonAuthRequest(http.MethodGet, "/api/clients/"+intToString(env.client.ID)+"/links", env.adminToken, nil)
	listReq.Host = "tenant.example.test"
	listResp, err := env.app.Test(listReq)
	if err != nil {
		t.Fatalf("list links request failed: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listedLinks []models.SecureLink
	if err := json.NewDecoder(listResp.Body).Decode(&listedLinks); err != nil {
		t.Fatalf("failed decoding listed links: %v", err)
	}
	if len(listedLinks) == 0 || listedLinks[0].LinkURL != createdLink.LinkURL {
		t.Fatalf("expected listed secure links to include link_url %q", createdLink.LinkURL)
	}

	viewReq := jsonAuthRequest(http.MethodGet, "/api/link/"+createdLink.Token, env.adminToken, nil)
	viewResp, err := env.app.Test(viewReq)
	if err != nil {
		t.Fatalf("view link request failed: %v", err)
	}
	if viewResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", viewResp.StatusCode)
	}
	viewCookie := viewResp.Header.Get("Set-Cookie")
	if !strings.Contains(viewCookie, "jwt=") {
		t.Fatalf("expected view link to set jwt cookie, got %q", viewCookie)
	}
	var viewBody map[string]any
	if err := json.NewDecoder(viewResp.Body).Decode(&viewBody); err != nil {
		t.Fatalf("decode view link failed: %v", err)
	}
	if _, ok := viewBody["token"]; ok {
		t.Fatalf("expected view link response without token, got %#v", viewBody)
	}

	bootstrapReq := jsonRequest(http.MethodGet, "/links/"+createdLink.Token, nil)
	bootstrapResp, err := env.app.Test(bootstrapReq)
	if err != nil {
		t.Fatalf("bootstrap request failed: %v", err)
	}
	if bootstrapResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", bootstrapResp.StatusCode)
	}
	bodyBytes, _ := io.ReadAll(bootstrapResp.Body)
	if !strings.Contains(string(bodyBytes), "/folder/") {
		t.Fatalf("expected bootstrap html redirect to folder")
	}
	if strings.Contains(string(bodyBytes), "localStorage.setItem('jwt'") {
		t.Fatalf("expected bootstrap html to avoid storing jwt in localStorage")
	}

	deleteReq := jsonAuthRequest(http.MethodDelete, "/api/links/"+intToString(createdLink.ID), env.adminToken, nil)
	deleteResp, err := env.app.Test(deleteReq)
	if err != nil {
		t.Fatalf("delete link request failed: %v", err)
	}
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteResp.StatusCode)
	}

	expiredToken, err := links.GenerateSecureToken(env.signingKey)
	if err != nil {
		t.Fatalf("failed generating expired token: %v", err)
	}
	if err := env.db.Create(&models.SecureLink{ClientID: env.client.ID, Token: expiredToken, ExpiresAt: time.Now().Add(-time.Hour), AccessType: "view", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("failed creating expired link: %v", err)
	}
	expiredViewReq := jsonAuthRequest(http.MethodGet, "/api/link/"+expiredToken, env.adminToken, nil)
	expiredViewResp, err := env.app.Test(expiredViewReq)
	if err != nil {
		t.Fatalf("expired view request failed: %v", err)
	}
	if expiredViewResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", expiredViewResp.StatusCode)
	}
}

func TestCreateSecureLinkRejectsInvalidEmail(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	req := jsonAuthRequest(http.MethodPost, "/api/clients/"+intToString(env.client.ID)+"/links", env.adminToken, map[string]any{"access_type": "view", "expiry_days": 1, "email": "not-an-email"})
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("create link request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
