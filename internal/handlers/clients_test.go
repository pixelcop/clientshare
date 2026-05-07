package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
)

type paginatedClientsResponse struct {
	Pagination models.PaginationResponse `json:"pagination"`
	Items      []models.Client           `json:"items"`
}

func TestClientHandlers_RouteCoverage(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	unauthReq := jsonRequest(http.MethodGet, "/api/clients", nil)
	unauthResp, err := env.app.Test(unauthReq)
	if err != nil {
		t.Fatalf("unauth request failed: %v", err)
	}
	if unauthResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthResp.StatusCode)
	}

	listReq := jsonAuthRequest(http.MethodGet, "/api/clients", env.managerToken, nil)
	listResp, err := env.app.Test(listReq)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listed []models.Client
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list failed: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected manager to see 2 assigned clients, got %d", len(listed))
	}
	if listed[0].ID != env.client.ID {
		t.Fatalf("expected manager list client %s, got %s", env.client.ID, listed[0].ID)
	}

	clientListReq := jsonAuthRequest(http.MethodGet, "/api/clients", env.clientToken, nil)
	clientListResp, err := env.app.Test(clientListReq)
	if err != nil {
		t.Fatalf("client list request failed: %v", err)
	}
	if clientListResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", clientListResp.StatusCode)
	}
	var clientListed []models.Client
	if err := json.NewDecoder(clientListResp.Body).Decode(&clientListed); err != nil {
		t.Fatalf("decode client list failed: %v", err)
	}
	if len(clientListed) != 1 || clientListed[0].ID != env.client.ID {
		t.Fatalf("expected client user to only see assigned client %s", env.client.ID)
	}

	pagedReq := jsonAuthRequest(http.MethodGet, "/api/clients?page=1&page_size=1&search=Ac", env.managerToken, nil)
	pagedResp, err := env.app.Test(pagedReq)
	if err != nil {
		t.Fatalf("paginated list request failed: %v", err)
	}
	if pagedResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", pagedResp.StatusCode)
	}
	var paged paginatedClientsResponse
	if err := json.NewDecoder(pagedResp.Body).Decode(&paged); err != nil {
		t.Fatalf("decode paginated list failed: %v", err)
	}
	if paged.Pagination.TotalItems != 1 {
		t.Fatalf("expected 1 total item in paginated response, got %d", paged.Pagination.TotalItems)
	}
	if len(paged.Items) != 1 || paged.Items[0].ID != env.client.ID {
		t.Fatalf("expected assigned client %s in paginated results", env.client.ID)
	}

	createBadReq := jsonAuthRequest(http.MethodPost, "/api/clients", env.managerToken, map[string]any{"name": ""})
	createBadResp, err := env.app.Test(createBadReq)
	if err != nil {
		t.Fatalf("create bad request failed: %v", err)
	}
	if createBadResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", createBadResp.StatusCode)
	}

	createReq := jsonAuthRequest(http.MethodPost, "/api/clients", env.managerToken, map[string]any{"name": "BetaCorp"})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	var created models.Client
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}

	getReq := jsonAuthRequest(http.MethodGet, "/api/clients/"+intToString(created.ID), env.managerToken, nil)
	getResp, err := env.app.Test(getReq)
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	updateBadReq := jsonAuthRequest(http.MethodPut, "/api/clients/"+intToString(created.ID), env.managerToken, map[string]any{"name": "trash"})
	updateBadResp, err := env.app.Test(updateBadReq)
	if err != nil {
		t.Fatalf("update bad request failed: %v", err)
	}
	if updateBadResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", updateBadResp.StatusCode)
	}

	deleteNoConfirm := jsonAuthRequest(http.MethodDelete, "/api/clients/"+intToString(created.ID), env.managerToken, nil)
	deleteNoConfirmResp, err := env.app.Test(deleteNoConfirm)
	if err != nil {
		t.Fatalf("delete without confirm failed: %v", err)
	}
	if deleteNoConfirmResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", deleteNoConfirmResp.StatusCode)
	}

	deleteReq := jsonAuthRequest(http.MethodDelete, "/api/clients/"+intToString(created.ID)+"?confirm=true", env.managerToken, nil)
	deleteResp, err := env.app.Test(deleteReq)
	if err != nil {
		t.Fatalf("delete request failed: %v", err)
	}
	if deleteResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteResp.StatusCode)
	}
}
