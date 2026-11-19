package handlers_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/stretchr/testify/require"
)

func TestDocumentRequestCollectionAndReview(t *testing.T) {
	env := setupSharedHandlersEnv(t)
	require.NoError(t, env.db.AutoMigrate(&models.DocumentRequest{}, &models.DocumentRequestItem{}, &models.DocumentRequestFile{}, &models.DocumentRequestReview{}))
	staff := "/api/clients/" + env.client.ID + "/document-requests"
	const recipient = "/api/document-request"

	do := func(req *http.Request, status int) []byte {
		t.Helper()
		res, err := env.app.Test(req)
		require.NoError(t, err)
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.Equal(t, status, res.StatusCode, string(data))
		require.Contains(t, res.Header.Get("Cache-Control"), "no-store")
		return data
	}
	decode := func(data []byte) services.DocumentRequestView {
		t.Helper()
		var view services.DocumentRequestView
		require.NoError(t, json.Unmarshal(data, &view))
		return view
	}

	input := `{"title":"October close","items":[{"name":"Bank statement","instructions":"Every page"},{"name":"Payroll summary"}]}`
	do(jsonAuthRequest("POST", staff, env.clientToken, json.RawMessage(input)), 404)
	do(jsonAuthRequest("POST", staff, env.linkToken, json.RawMessage(input)), 404)
	view := decode(do(jsonAuthRequest("POST", staff, env.managerToken, json.RawMessage(input)), 201))
	require.Len(t, view.Items, 2)
	bank, payroll := view.Items[0].ID, view.Items[1].ID
	link, err := url.Parse(view.Link)
	require.NoError(t, err)
	fragment, err := url.ParseQuery(link.Fragment)
	require.NoError(t, err)
	token := fragment.Get("token")
	require.NotEmpty(t, token)
	staff += "/" + view.ID
	public := func(method, path, jsonBody string) *http.Request {
		req := httptest.NewRequest(method, path, strings.NewReader(jsonBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Document-Request", token)
		return req
	}
	upload := func(item, round, name string, status int) {
		t.Helper()
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		require.NoError(t, writer.WriteField("round", round))
		part, err := writer.CreateFormFile("file", name)
		require.NoError(t, err)
		_, err = io.WriteString(part, "sample file material")
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		req := httptest.NewRequest("POST", recipient+"/items/"+item+"/files", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-Document-Request", token)
		do(req, status)
	}
	review := func(item, body string, status int) {
		t.Helper()
		do(jsonAuthRequest("POST", staff+"/items/"+item+"/review", env.adminToken, json.RawMessage(body)), status)
	}

	do(httptest.NewRequest("GET", recipient, nil), 404)
	tampered := public("GET", recipient, "")
	tampered.Header.Set("X-Document-Request", token+"X")
	do(tampered, 404)
	do(jsonAuthRequest("GET", "/api/clients/"+env.otherClient.ID+"/document-requests/"+view.ID, env.adminToken, nil), 404)
	do(public("GET", "/api/folders", ""), 401)
	do(public("POST", recipient+"/items/"+bank+"/submit", `{"round":1}`), 409)
	upload("unknown-item", "1", "wrong.txt", 404)
	upload(bank, "1", "statement.txt", 201)
	upload(payroll, "1", "payroll.txt", 201)

	saved := decode(do(public("GET", recipient, ""), 200))
	require.Empty(t, saved.Link)
	require.Empty(t, saved.ID)
	require.Empty(t, saved.Items[0].Files[0].ID)
	require.Equal(t, "needed", saved.Items[0].Status, "upload is not submission")
	require.Equal(t, "statement.txt", saved.Items[0].Files[0].Name)
	do(public("POST", recipient+"/items/"+bank+"/submit", `{"round":1}`), 204)
	do(public("POST", recipient+"/items/"+payroll+"/submit", `{"round":1}`), 204)
	upload(bank, "1", "late.txt", 409)
	review(payroll, `{"round":1,"status":"accepted"}`, 204)
	review(bank, `{"round":1,"status":"replacement"}`, 400)
	review(bank, `{"round":1,"status":"replacement","reason":"Page 2 is missing"}`, 204)
	returned := decode(do(public("GET", recipient, ""), 200))
	require.Equal(t, "Page 2 is missing", returned.Items[0].Reason)
	require.Empty(t, returned.Items[0].Files)
	require.Empty(t, returned.Items[0].History)
	require.Equal(t, "accepted", returned.Items[1].Status)
	upload(bank, "1", "stale.txt", 409)
	upload(bank, "2", "complete.txt", 201)
	do(public("POST", recipient+"/items/"+bank+"/submit", `{"round":2}`), 204)
	review(bank, `{"round":1,"status":"accepted"}`, 409)
	review(bank, `{"round":2,"status":"accepted"}`, 204)

	detail := decode(do(jsonAuthRequest("GET", staff, env.adminToken, nil), 200))
	require.Len(t, detail.Items[0].History, 2)
	require.Len(t, detail.Items[0].Files, 2)
	fileID := detail.Items[0].Files[0].ID
	require.NotEmpty(t, fileID)
	do(public("GET", "/api/files/"+fileID, ""), 401)
	data := do(jsonAuthRequest("GET", "/api/files/"+fileID+"/download", env.managerToken, nil), 200)
	require.Equal(t, "sample file material", string(data))
	require.NoError(t, env.db.Model(&models.DocumentRequest{}).Where("id = ?", view.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	do(public("GET", recipient, ""), 404)
	require.NoError(t, env.db.Model(&models.DocumentRequest{}).Where("id = ?", view.ID).Update("expires_at", time.Now().Add(time.Hour)).Error)
	do(jsonAuthRequest("POST", staff+"/revoke", env.adminToken, nil), 204)
	do(public("GET", recipient, ""), 404)
	detail = decode(do(jsonAuthRequest("GET", staff, env.adminToken, nil), 200))
	require.True(t, detail.Revoked)
	require.Empty(t, detail.Link)
}
