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
	"github.com/pixelcop/clientshare/internal/services/email"
	"github.com/pixelcop/clientshare/internal/services/storage"
	"github.com/stretchr/testify/require"
)

func TestDocumentRequestCollectionAndReview(t *testing.T) {
	env := setupSharedHandlersEnv(t)
	require.NoError(t, env.db.AutoMigrate(&models.DocumentRequest{}, &models.DocumentRequestItem{}, &models.DocumentRequestFile{}, &models.DocumentRequestReview{}, &models.DocumentRequestUpdate{}))
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

	input := `{"title":"October close","description":"Please use PDFs if possible.","recipient_emails":["` + env.clientUser.Email + `"],"items":[{"name":"Bank statement","instructions":"Every page"},{"name":"Payroll summary"}]}`
	do(jsonAuthRequest("POST", staff, env.clientToken, json.RawMessage(input)), 404)
	do(jsonAuthRequest("POST", staff, env.linkToken, json.RawMessage(input)), 404)
	view := decode(do(jsonAuthRequest("POST", staff, env.managerToken, json.RawMessage(input)), 201))
	require.Len(t, view.Items, 2)
	require.Equal(t, "October close", view.FolderName)
	require.Equal(t, "Please use PDFs if possible.", view.Description)
	require.NotEmpty(t, view.FolderID)
	do(jsonAuthRequest("POST", staff, env.managerToken, json.RawMessage(input)), 409)
	override := `{"title":"October close","folder_name":"2026 October uploads","items":[{"name":"Another file"}]}`
	second := decode(do(jsonAuthRequest("POST", staff, env.managerToken, json.RawMessage(override)), 201))
	require.Equal(t, "2026 October uploads", second.FolderName)
	bank, payroll := view.Items[0].ID, view.Items[1].ID
	activity := func(session string, expected ...string) {
		t.Helper()
		res := do(jsonAuthRequest("GET", "/api/feed?state=all", session, nil), 200)
		require.NotContains(t, string(res), "token=")
		require.NotContains(t, string(res), "Page 2 is missing")
		var result models.FeedListResponse
		require.NoError(t, json.Unmarshal(res, &result))
		actual := make([]string, 0, len(result.Items))
		for _, event := range result.Items {
			actual = append(actual, event.EventType)
			require.Equal(t, view.ID, event.RequestID)
			require.Equal(t, "October close", event.RequestTitle)
		}
		require.ElementsMatch(t, expected, actual)
	}
	activity(env.clientToken, models.FeedEventTypeRequestCreated)
	activity(env.mismatchToken)
	adminEvents := do(jsonAuthRequest("GET", "/api/feed", env.adminToken, nil), 200)
	require.Contains(t, string(adminEvents), models.FeedEventSourceTypeDocumentRequestStaff)
	require.Contains(t, string(adminEvents), view.ID)
	accountRequests := func(session string, expected ...string) {
		t.Helper()
		raw := do(jsonAuthRequest("GET", "/api/my-document-requests", session, nil), 200)
		var list []services.AccountDocumentRequest
		require.NoError(t, json.Unmarshal(raw, &list))
		got := make([]string, 0, len(list))
		for _, entry := range list {
			got = append(got, entry.ID)
		}
		require.ElementsMatch(t, expected, got)
		require.NotContains(t, string(raw), "token")
	}
	accountRequests(env.clientToken, view.ID)
	accountRequests(env.mismatchToken)
	accountRequests(env.otherToken)
	do(jsonAuthRequest("GET", "/api/my-document-requests", env.linkToken, nil), 404)
	accountPath := "/api/my-document-requests/" + view.ID + "/link"
	do(jsonAuthRequest("GET", accountPath, env.clientToken, nil), 200)
	do(jsonAuthRequest("GET", accountPath, env.mismatchToken, nil), 404)
	do(jsonAuthRequest("GET", accountPath, env.otherToken, nil), 404)
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
	activity(env.managerToken)

	saved := decode(do(public("GET", recipient, ""), 200))
	require.Empty(t, saved.Link)
	require.Empty(t, saved.ID)
	require.Empty(t, saved.FolderID)
	require.Empty(t, saved.Items[0].Files[0].ID)
	require.Equal(t, "needed", saved.Items[0].Status, "upload is not submission")
	require.Equal(t, "statement.txt", saved.Items[0].Files[0].Name)
	do(public("POST", recipient+"/items/"+bank+"/submit", `{"round":1}`), 204)
	do(public("POST", recipient+"/items/"+bank+"/submit", `{"round":1}`), 204)
	do(public("POST", recipient+"/items/"+payroll+"/submit", `{"round":1}`), 204)
	activity(env.managerToken, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestSubmitted)
	upload(bank, "1", "late.txt", 409)
	review(payroll, `{"round":1,"status":"accepted"}`, 204)
	review(bank, `{"round":1,"status":"replacement"}`, 400)
	review(bank, `{"round":1,"status":"replacement","reason":"Page 2 is missing"}`, 204)
	activity(env.clientToken, models.FeedEventTypeRequestCreated, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestAccepted, models.FeedEventTypeRequestReplacement)
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
	var file models.File
	require.NoError(t, env.db.First(&file, "id = ?", fileID).Error)
	require.NotNil(t, file.FolderID)
	require.Equal(t, view.FolderID, *file.FolderID)
	do(public("GET", "/api/files/"+fileID, ""), 401)
	data := do(jsonAuthRequest("GET", "/api/files/"+fileID+"/download", env.managerToken, nil), 200)
	require.Equal(t, "sample file material", string(data))
	addition := json.RawMessage(`{"description":"Please also include October invoices.","items":[{"name":"October invoices"}]}`)
	do(jsonAuthRequest("POST", staff+"/additions", env.clientToken, addition), 404)
	expanded := decode(do(jsonAuthRequest("POST", staff+"/additions", env.adminToken, addition), 201))
	require.Equal(t, view.Link, expanded.Link)
	require.Equal(t, view.FolderID, expanded.FolderID)
	expanded = decode(do(public("GET", recipient, ""), 200))
	require.Equal(t, "Please use PDFs if possible.", expanded.Description)
	require.Len(t, expanded.Updates, 1)
	require.Equal(t, "Please also include October invoices.", expanded.Updates[0].Description)
	require.Len(t, expanded.Items, 3)
	require.Equal(t, "accepted", expanded.Items[0].Status)
	require.Equal(t, "accepted", expanded.Items[1].Status)
	require.Equal(t, "needed", expanded.Items[2].Status)
	require.Equal(t, "October invoices", expanded.Items[2].Name)
	activity(env.clientToken, models.FeedEventTypeRequestCreated, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestAccepted, models.FeedEventTypeRequestReplacement, models.FeedEventTypeRequestAccepted, models.FeedEventTypeRequestUpdated)
	activity(env.managerToken, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestSubmitted, models.FeedEventTypeRequestAccepted, models.FeedEventTypeRequestReplacement, models.FeedEventTypeRequestAccepted, models.FeedEventTypeRequestUpdated)
	activity(env.mismatchToken)
	require.NoError(t, env.db.Model(&models.DocumentRequest{}).Where("id = ?", view.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	do(public("GET", recipient, ""), 404)
	accountRequests(env.clientToken)
	require.NoError(t, env.db.Model(&models.DocumentRequest{}).Where("id = ?", view.ID).Update("expires_at", time.Now().Add(time.Hour)).Error)
	do(jsonAuthRequest("POST", staff+"/revoke", env.adminToken, nil), 204)
	do(jsonAuthRequest("POST", staff+"/additions", env.adminToken, addition), 404)
	do(public("GET", recipient, ""), 404)
	do(jsonAuthRequest("GET", accountPath, env.clientToken, nil), 404)
	activity(env.clientToken)
	accountRequests(env.clientToken)
	detail = decode(do(jsonAuthRequest("GET", staff, env.adminToken, nil), 200))
	require.True(t, detail.Revoked)
	require.Empty(t, detail.Link)
}

type documentRequestSender struct{ messages []email.Email }

func (s *documentRequestSender) Send(msg email.Email) error {
	s.messages = append(s.messages, msg)
	return nil
}

func TestDocumentRequestEmailsOnlySelectedEligibleRecipient(t *testing.T) {
	env := setupSharedHandlersEnv(t)
	require.NoError(t, env.db.AutoMigrate(&models.DocumentRequest{}, &models.DocumentRequestItem{}, &models.DocumentRequestFile{}, &models.DocumentRequestReview{}, &models.DocumentRequestUpdate{}))
	svc := &services.DocumentRequestService{DB: env.db, Store: storage.NewLocalStorage(env.storageRoot), SigningKey: env.signingKey, NotificationBaseURL: "https://portal.example.test"}
	req := services.AccessRequest{
		TenantID: env.client.TenantID, ClientID: env.client.ID,
		Actor: services.AccessActor{UserID: env.adminUser.ID},
	}
	in := services.DocumentRequestInput{
		Title: "Bank documents", Description: "Use the final statement.", RecipientEmails: []string{env.otherClientUser.Email},
		Items: []services.DocumentRequestItemInput{{Name: "Bank statement"}},
	}
	_, err := svc.Create(t.Context(), req, in)
	require.ErrorIs(t, err, services.ErrDocumentRequestRecipient)
	in.RecipientEmails = []string{env.clientUser.Email, strings.ToUpper(env.clientUser.Email)}
	result, err := svc.Create(t.Context(), req, in)
	require.NoError(t, err)
	require.Equal(t, "queued", result.EmailStatus)
	var items []models.QueuedEmail
	require.NoError(t, env.db.Where("document_request_id = ?", result.ID).Find(&items).Error)
	require.Len(t, items, 1)
	require.Equal(t, env.clientUser.Email, items[0].Recipient)
	require.Empty(t, items[0].Body, "bearer link is only composed for delivery")

	sender := &documentRequestSender{}
	queue := email.NewDBEmailQueue(env.db, sender, time.Hour)
	queue.DocumentRequestSigningKey = env.signingKey
	require.Error(t, queue.Update(req.TenantID, items[0].ID, env.otherClientUser.Email, "Other message", "Other body"))
	require.NoError(t, queue.SendNow(req.TenantID, items[0].ID))
	require.Len(t, sender.messages, 1)
	require.Contains(t, sender.messages[0].Body, "https://portal.example.test"+result.Link)
	require.Contains(t, sender.messages[0].Body, in.Description)
	update, err := svc.AddItems(t.Context(), req, result.ID, services.DocumentRequestAddition{
		Description: "Include the full invoice.",
		Items:       []services.DocumentRequestItemInput{{Name: "Invoice"}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, update.EmailCount)
	var updateEmail models.QueuedEmail
	require.NoError(t, env.db.Where("document_request_id = ? AND document_request_update_id IS NOT NULL", result.ID).First(&updateEmail).Error)
	require.Empty(t, updateEmail.Body)
	require.NoError(t, queue.SendNow(req.TenantID, updateEmail.ID))
	require.Len(t, sender.messages, 2)
	require.Equal(t, "More documents requested", sender.messages[1].Subject)
	require.Contains(t, sender.messages[1].Body, "Include the full invoice.")
	require.NotContains(t, sender.messages[1].Body, in.Description)
	require.Contains(t, sender.messages[1].Body, "https://portal.example.test"+result.Link)

	in.FolderName = "Second bank request"
	in.EmailAll, in.RecipientEmails = true, nil
	result, err = svc.Create(t.Context(), req, in)
	require.NoError(t, err)
	require.Equal(t, 2, result.EmailCount)
	var all []string
	require.NoError(t, env.db.Model(&models.QueuedEmail{}).Where("document_request_id = ?", result.ID).Pluck("recipient", &all).Error)
	require.ElementsMatch(t, []string{env.clientUser.Email, env.mismatchUser.Email}, all)
	var notified []string
	require.NoError(t, env.db.Model(&models.FeedEvent{}).Where("event_type = ? AND source_type = ? AND source_id = ?", models.FeedEventTypeRequestCreated, models.FeedEventSourceTypeDocumentRequest, result.ID).Pluck("user_id", &notified).Error)
	require.ElementsMatch(t, []string{env.clientUser.ID, env.mismatchUser.ID}, notified)
	list, err := svc.AccountRequests(t.Context(), req.TenantID, env.mismatchUser.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, result.ID, list[0].ID)
	require.Equal(t, 1, list[0].NeedsAction)
	require.Zero(t, list[0].UnderReview)
	var canceled models.QueuedEmail
	require.NoError(t, env.db.Where("document_request_id = ?", result.ID).First(&canceled).Error)
	require.NoError(t, svc.Revoke(t.Context(), req, result.ID))
	require.NoError(t, queue.SendNow(req.TenantID, canceled.ID))
	require.Len(t, sender.messages, 2)
	require.NoError(t, env.db.First(&canceled, "id = ?", canceled.ID).Error)
	require.NotNil(t, canceled.CanceledAt)
}
