package handlers_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
)

type feedResponse struct {
	Pagination models.PaginationResponse `json:"pagination"`
	Items      []models.FeedEvent        `json:"items"`
	Summary    models.FeedSummary        `json:"summary"`
}

func TestFeedListsUploadEventsByRole(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "feed-note.txt")
	if err != nil {
		t.Fatalf("failed to create upload part: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("failed to write upload content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	uploadReq := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := env.app.Test(uploadReq)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d", uploadResp.StatusCode)
	}

	var uploadedFile models.File
	if err := env.db.Where("filename = ?", "feed-note.txt").First(&uploadedFile).Error; err != nil {
		t.Fatalf("expected uploaded file record: %v", err)
	}

	adminResp := requestFeed(t, env, env.adminToken, models.FeedStateUnread)
	if len(adminResp.Items) != 0 {
		t.Fatalf("expected uploader admin to see 0 feed items, got %d", len(adminResp.Items))
	}
	if adminResp.Summary.UnreadCount != 0 || adminResp.Summary.ReadCount != 0 {
		t.Fatalf("expected uploader admin unread/read summary 0/0, got %d/%d", adminResp.Summary.UnreadCount, adminResp.Summary.ReadCount)
	}

	managerResp := requestFeed(t, env, env.managerToken, models.FeedStateUnread)
	if len(managerResp.Items) != 1 {
		t.Fatalf("expected manager to see 1 feed item, got %d", len(managerResp.Items))
	}
	if managerResp.Items[0].EventType != models.FeedEventTypeFileUploaded {
		t.Fatalf("expected upload event, got %q", managerResp.Items[0].EventType)
	}
	if managerResp.Items[0].ClientName != env.client.Name {
		t.Fatalf("expected client name %q, got %q", env.client.Name, managerResp.Items[0].ClientName)
	}
	if managerResp.Items[0].ActorEmail != env.adminUser.Email {
		t.Fatalf("expected actor email %q, got %q", env.adminUser.Email, managerResp.Items[0].ActorEmail)
	}
	if !strings.Contains(managerResp.Items[0].FilePath, "feed-note.txt") {
		t.Fatalf("expected file path to include uploaded filename, got %q", managerResp.Items[0].FilePath)
	}
	if managerResp.Items[0].FileID == nil || *managerResp.Items[0].FileID != uploadedFile.ID {
		t.Fatalf("expected feed response file_id %q, got %v", uploadedFile.ID, managerResp.Items[0].FileID)
	}
	if managerResp.Items[0].IsRead {
		t.Fatalf("expected new upload event to be unread")
	}
	if managerResp.Summary.UnreadCount != 1 || managerResp.Summary.ReadCount != 0 {
		t.Fatalf("expected unread/read summary 1/0, got %d/%d", managerResp.Summary.UnreadCount, managerResp.Summary.ReadCount)
	}

	clientResp := requestFeed(t, env, env.clientToken, models.FeedStateUnread)
	if len(clientResp.Items) != 1 {
		t.Fatalf("expected assigned client user to see 1 feed item, got %d", len(clientResp.Items))
	}

	otherClientResp := requestFeed(t, env, env.otherToken, models.FeedStateUnread)
	if len(otherClientResp.Items) != 0 {
		t.Fatalf("expected unrelated client user to see 0 feed items, got %d", len(otherClientResp.Items))
	}
	if otherClientResp.Pagination.TotalItems != 0 {
		t.Fatalf("expected unrelated client total_items 0, got %d", otherClientResp.Pagination.TotalItems)
	}

	var storedCount int64
	if err := env.db.Model(&models.FeedEvent{}).Where("event_type = ?", models.FeedEventTypeFileUploaded).Count(&storedCount).Error; err != nil {
		t.Fatalf("expected stored upload feed events: %v", err)
	}
	if storedCount != 3 {
		t.Fatalf("expected 3 stored upload feed events, got %d", storedCount)
	}

	var stored models.FeedEvent
	if err := env.db.Where("event_type = ? AND user_id = ?", models.FeedEventTypeFileUploaded, env.managerUser.ID).First(&stored).Error; err != nil {
		t.Fatalf("expected stored manager upload feed event: %v", err)
	}
	if stored.SourceType != models.FeedEventSourceTypeFileActivity {
		t.Fatalf("expected source_type %q, got %q", models.FeedEventSourceTypeFileActivity, stored.SourceType)
	}
	if stored.FileID == nil || *stored.FileID != uploadedFile.ID {
		t.Fatalf("expected stored file_id %q, got %v", uploadedFile.ID, stored.FileID)
	}
}

func TestFeedInviteAcceptedVisibleOnlyToAdmins(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	createReq := jsonAuthRequest(http.MethodPost, "/api/users", env.adminToken, map[string]any{
		"email":      "invited-feed@test.local",
		"name":       "Invited Feed User",
		"role":       "client",
		"client_ids": []string{env.client.ID},
	})
	createResp, err := env.app.Test(createReq)
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected create user 201, got %d", createResp.StatusCode)
	}

	inviteBody := env.queue.items[len(env.queue.items)-1].Body
	start := "#token="
	idx := strings.Index(inviteBody, start)
	if idx == -1 {
		t.Fatalf("expected invite body to include token")
	}
	token := strings.Fields(inviteBody[idx+len(start):])[0]

	acceptReq := jsonRequest(http.MethodPost, "/api/auth/accept-invite", map[string]any{
		"token":    token,
		"password": "invite-pass-456",
	})
	acceptResp, err := env.app.Test(acceptReq)
	if err != nil {
		t.Fatalf("accept invite request failed: %v", err)
	}
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("expected accept invite 200, got %d", acceptResp.StatusCode)
	}

	adminResp := requestFeed(t, env, env.adminToken, models.FeedStateUnread)
	if len(adminResp.Items) == 0 {
		t.Fatalf("expected admin feed to include invite accepted event")
	}
	if adminResp.Items[0].EventType != models.FeedEventTypeInviteAccepted {
		t.Fatalf("expected newest admin feed item to be invite accepted, got %q", adminResp.Items[0].EventType)
	}
	if adminResp.Items[0].SubjectEmail != "invited-feed@test.local" {
		t.Fatalf("expected subject email invited-feed@test.local, got %q", adminResp.Items[0].SubjectEmail)
	}

	managerResp := requestFeed(t, env, env.managerToken, models.FeedStateUnread)
	for _, item := range managerResp.Items {
		if item.EventType == models.FeedEventTypeInviteAccepted {
			t.Fatalf("manager feed should not include invite accepted events")
		}
	}

	clientResp := requestFeed(t, env, env.clientToken, models.FeedStateUnread)
	for _, item := range clientResp.Items {
		if item.EventType == models.FeedEventTypeInviteAccepted {
			t.Fatalf("client feed should not include invite accepted events")
		}
	}
}

func TestFeedMarksReadPerUser(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files", "read-state-note.txt")
	if err != nil {
		t.Fatalf("failed to create upload part: %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("failed to write upload content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	uploadReq := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := env.app.Test(uploadReq)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d", uploadResp.StatusCode)
	}

	adminUnread := requestFeed(t, env, env.adminToken, models.FeedStateUnread)
	if len(adminUnread.Items) != 0 {
		t.Fatalf("expected uploader admin to have 0 unread events, got %d", len(adminUnread.Items))
	}

	managerUnread := requestFeed(t, env, env.managerToken, models.FeedStateUnread)
	if len(managerUnread.Items) != 1 {
		t.Fatalf("expected manager to have 1 unread event, got %d", len(managerUnread.Items))
	}
	eventID := managerUnread.Items[0].ID

	markReq := jsonAuthRequest(http.MethodPost, "/api/feed/"+eventID+"/read", env.managerToken, nil)
	markResp, err := env.app.Test(markReq)
	if err != nil {
		t.Fatalf("mark read request failed: %v", err)
	}
	if markResp.StatusCode != http.StatusOK {
		t.Fatalf("expected mark read 200, got %d", markResp.StatusCode)
	}

	managerUnread = requestFeed(t, env, env.managerToken, models.FeedStateUnread)
	if len(managerUnread.Items) != 0 {
		t.Fatalf("expected 0 unread events after mark read, got %d", len(managerUnread.Items))
	}
	if managerUnread.Summary.UnreadCount != 0 || managerUnread.Summary.ReadCount != 1 {
		t.Fatalf("expected unread/read summary 0/1 after mark read, got %d/%d", managerUnread.Summary.UnreadCount, managerUnread.Summary.ReadCount)
	}

	managerRead := requestFeed(t, env, env.managerToken, models.FeedStateRead)
	if len(managerRead.Items) != 1 {
		t.Fatalf("expected 1 read event, got %d", len(managerRead.Items))
	}
	if !managerRead.Items[0].IsRead {
		t.Fatalf("expected read tab item to be marked read")
	}
	if managerRead.Items[0].ReadAt == nil {
		t.Fatalf("expected read tab item to include read_at timestamp")
	}

	clientUnread := requestFeed(t, env, env.clientToken, models.FeedStateUnread)
	if len(clientUnread.Items) != 1 {
		t.Fatalf("expected client unread feed to remain unaffected, got %d", len(clientUnread.Items))
	}
	if clientUnread.Items[0].IsRead {
		t.Fatalf("expected client event to remain unread")
	}

	adminRead := requestFeed(t, env, env.adminToken, models.FeedStateRead)
	if len(adminRead.Items) != 0 {
		t.Fatalf("expected uploader admin read feed to remain empty, got %d", len(adminRead.Items))
	}
}

func TestFeedMarksAllReadPerUser(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	for _, name := range []string{"bulk-read-1.txt", "bulk-read-2.txt"} {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("failed to create upload part %s: %v", name, err)
		}
		if _, err := part.Write([]byte("hello")); err != nil {
			t.Fatalf("failed to write upload content %s: %v", name, err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("failed to close multipart writer for %s: %v", name, err)
		}

		uploadReq := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, body)
		uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
		uploadResp, err := env.app.Test(uploadReq)
		if err != nil {
			t.Fatalf("upload request %s failed: %v", name, err)
		}
		if uploadResp.StatusCode != http.StatusCreated {
			t.Fatalf("expected upload 201 for %s, got %d", name, uploadResp.StatusCode)
		}
	}

	managerUnread := requestFeed(t, env, env.managerToken, models.FeedStateUnread)
	if len(managerUnread.Items) != 2 {
		t.Fatalf("expected manager to have 2 unread events before clear all, got %d", len(managerUnread.Items))
	}
	if managerUnread.Summary.UnreadCount != 2 || managerUnread.Summary.ReadCount != 0 {
		t.Fatalf("expected unread/read summary 2/0 before clear all, got %d/%d", managerUnread.Summary.UnreadCount, managerUnread.Summary.ReadCount)
	}

	markReq := jsonAuthRequest(http.MethodPost, "/api/feed/read-all", env.managerToken, nil)
	markResp, err := env.app.Test(markReq)
	if err != nil {
		t.Fatalf("mark all read request failed: %v", err)
	}
	if markResp.StatusCode != http.StatusOK {
		t.Fatalf("expected mark all read 200, got %d", markResp.StatusCode)
	}

	managerUnread = requestFeed(t, env, env.managerToken, models.FeedStateUnread)
	if len(managerUnread.Items) != 0 {
		t.Fatalf("expected 0 unread events after clear all, got %d", len(managerUnread.Items))
	}
	if managerUnread.Summary.UnreadCount != 0 || managerUnread.Summary.ReadCount != 2 {
		t.Fatalf("expected unread/read summary 0/2 after clear all, got %d/%d", managerUnread.Summary.UnreadCount, managerUnread.Summary.ReadCount)
	}

	managerRead := requestFeed(t, env, env.managerToken, models.FeedStateRead)
	if len(managerRead.Items) != 2 {
		t.Fatalf("expected 2 read events after clear all, got %d", len(managerRead.Items))
	}
	for _, item := range managerRead.Items {
		if !item.IsRead {
			t.Fatalf("expected cleared feed items to be marked read")
		}
		if item.ReadAt == nil {
			t.Fatalf("expected cleared feed items to include read_at timestamp")
		}
	}

	clientUnread := requestFeed(t, env, env.clientToken, models.FeedStateUnread)
	if len(clientUnread.Items) != 2 {
		t.Fatalf("expected client unread feed to remain unaffected, got %d", len(clientUnread.Items))
	}
	for _, item := range clientUnread.Items {
		if item.IsRead {
			t.Fatalf("expected client feed items to remain unread")
		}
	}
}

func TestFeedGroupsMultiFileUploadsIntoSingleEvent(t *testing.T) {
	env := setupOtherHandlersEnv(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for _, name := range []string{"alpha.txt", "beta.txt"} {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("failed to create upload part %s: %v", name, err)
		}
		if _, err := part.Write([]byte("hello")); err != nil {
			t.Fatalf("failed to write upload content %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	uploadReq := newAuthRequest(http.MethodPost, "/api/folders/"+env.rootFolder.ID+"/upload", env.adminToken, body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadResp, err := env.app.Test(uploadReq)
	if err != nil {
		t.Fatalf("upload request failed: %v", err)
	}
	if uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected upload 201, got %d", uploadResp.StatusCode)
	}

	adminResp := requestFeed(t, env, env.adminToken, models.FeedStateUnread)
	if len(adminResp.Items) != 0 {
		t.Fatalf("expected uploader admin to see 0 grouped feed items, got %d", len(adminResp.Items))
	}

	managerResp := requestFeed(t, env, env.managerToken, models.FeedStateUnread)
	if len(managerResp.Items) != 1 {
		t.Fatalf("expected manager to see 1 grouped feed item, got %d", len(managerResp.Items))
	}
	groupedItem := managerResp.Items[0]
	if groupedItem.SourceType != models.FeedEventSourceTypeFileUploadBatch {
		t.Fatalf("expected grouped upload source type %q, got %q", models.FeedEventSourceTypeFileUploadBatch, groupedItem.SourceType)
	}
	if groupedItem.FileCount != 2 {
		t.Fatalf("expected grouped upload file_count 2, got %d", groupedItem.FileCount)
	}
	if len(groupedItem.Files) != 2 {
		t.Fatalf("expected grouped upload files length 2, got %d", len(groupedItem.Files))
	}
	if !strings.Contains(groupedItem.Files[0].Path, "alpha.txt") {
		t.Fatalf("expected first grouped file path to include alpha.txt, got %q", groupedItem.Files[0].Path)
	}
	if !strings.Contains(groupedItem.Files[1].Path, "beta.txt") {
		t.Fatalf("expected second grouped file path to include beta.txt, got %q", groupedItem.Files[1].Path)
	}

	clientResp := requestFeed(t, env, env.clientToken, models.FeedStateUnread)
	if len(clientResp.Items) != 1 {
		t.Fatalf("expected assigned client user to see 1 grouped feed item, got %d", len(clientResp.Items))
	}

	var uploadActivities []models.FileActivity
	if err := env.db.Where("action = ?", "upload").Order("created_at ASC, id ASC").Find(&uploadActivities).Error; err != nil {
		t.Fatalf("expected upload activities: %v", err)
	}
	if len(uploadActivities) != 2 {
		t.Fatalf("expected 2 upload activity rows, got %d", len(uploadActivities))
	}
	if uploadActivities[0].UploadBatchID == nil || strings.TrimSpace(*uploadActivities[0].UploadBatchID) == "" {
		t.Fatalf("expected first upload activity to have an upload batch id")
	}
	if uploadActivities[1].UploadBatchID == nil || *uploadActivities[1].UploadBatchID != *uploadActivities[0].UploadBatchID {
		t.Fatalf("expected upload activity rows to share the same upload batch id")
	}

	var storedCount int64
	if err := env.db.Model(&models.FeedEvent{}).Where("event_type = ?", models.FeedEventTypeFileUploaded).Count(&storedCount).Error; err != nil {
		t.Fatalf("expected stored upload feed events: %v", err)
	}
	if storedCount != 3 {
		t.Fatalf("expected 3 stored grouped upload feed events, got %d", storedCount)
	}

	markReq := jsonAuthRequest(http.MethodPost, "/api/feed/"+groupedItem.ID+"/read", env.managerToken, nil)
	markResp, err := env.app.Test(markReq)
	if err != nil {
		t.Fatalf("mark read request failed: %v", err)
	}
	if markResp.StatusCode != http.StatusOK {
		t.Fatalf("expected grouped mark read 200, got %d", markResp.StatusCode)
	}

	managerUnread := requestFeed(t, env, env.managerToken, models.FeedStateUnread)
	if len(managerUnread.Items) != 0 {
		t.Fatalf("expected grouped event to leave unread feed after mark read, got %d items", len(managerUnread.Items))
	}
	managerRead := requestFeed(t, env, env.managerToken, models.FeedStateRead)
	if len(managerRead.Items) != 1 {
		t.Fatalf("expected grouped event to appear once in read feed, got %d", len(managerRead.Items))
	}

	clientUnread := requestFeed(t, env, env.clientToken, models.FeedStateUnread)
	if len(clientUnread.Items) != 1 {
		t.Fatalf("expected client grouped unread feed to remain unaffected, got %d", len(clientUnread.Items))
	}
	if clientUnread.Items[0].IsRead {
		t.Fatalf("expected client grouped event to remain unread")
	}
}

func requestFeed(t *testing.T, env *otherHandlersEnv, token, state string) feedResponse {
	t.Helper()

	path := "/api/feed?page=1&page_size=20"
	if state != "" {
		path += "&state=" + state
	}
	req := jsonAuthRequest(http.MethodGet, path, token, nil)
	resp, err := env.app.Test(req)
	if err != nil {
		t.Fatalf("feed request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected feed 200, got %d", resp.StatusCode)
	}

	var payload feedResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode feed response: %v", err)
	}
	return payload
}
