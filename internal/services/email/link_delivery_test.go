package email

import (
	"sync"
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDurableEmailDeduplicationAndLinkRevalidation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/queue.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.QueuedEmail{}, &models.SecureLink{}, &models.Client{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	client := models.Client{TenantID: "tenant", Name: "Client"}
	require.NoError(t, db.Create(&client).Error)
	link := models.SecureLink{TenantID: "tenant", ClientID: client.ID, Token: "private-token", AccessType: "read", ExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, db.Create(&link).Error)
	sender := &headerSender{}
	queue := NewDBEmailQueue(db, sender, time.Hour)
	message := Email{TenantID: "tenant", To: "recipient@example.test", Subject: "Link", Body: "Link", SecureLinkID: &link.ID, DeduplicationKey: "scoped-request-digest"}
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := queue.Queue(message, models.Now()); errors <- err }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Model(&models.QueuedEmail{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.Empty(t, sender.messages, "queue acceptance must not imply delivery")
	require.NoError(t, queue.ProcessAll())
	require.Len(t, sender.messages, 1)
	_, err = NewDBEmailQueue(db, sender, time.Hour).Queue(message, models.Now())
	require.NoError(t, err)
	require.NoError(t, queue.ProcessAll())
	require.Len(t, sender.messages, 1)
	message.DeduplicationKey = "second-request-digest"
	_, err = queue.Queue(message, models.Now())
	require.NoError(t, err)
	require.NoError(t, db.Model(&link).Update("revoked_at", time.Now()).Error)
	require.NoError(t, queue.ProcessAll())
	require.Len(t, sender.messages, 1, "revoked links must not be emailed")
	var canceled models.QueuedEmail
	require.NoError(t, db.Where("deduplication_key = ?", message.DeduplicationKey).First(&canceled).Error)
	require.NotNil(t, canceled.CanceledAt)
	require.Nil(t, canceled.SentAt)
	// A UTC expiry that is earlier in absolute time but later as a wall-clock
	// string must still be rejected when the worker runs in another timezone.
	queue.Now = func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.FixedZone("EST", -5*3600)) }
	expired := models.SecureLink{TenantID: "tenant", ClientID: client.ID, Token: "expired-token", AccessType: "read", ExpiresAt: time.Date(2026, 11, 8, 16, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&expired).Error)
	message.SecureLinkID = &expired.ID
	message.DeduplicationKey = "expired-timezone-request"
	_, err = queue.Queue(message, models.Now())
	require.NoError(t, err)
	require.NoError(t, queue.ProcessAll())
	require.Len(t, sender.messages, 1, "an expired link must not be mailed across timestamp offsets")
}
