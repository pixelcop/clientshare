package email

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSQLiteEmailQueueSchedulesByInstantAcrossOffsets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/queue.db"), &gorm.Config{})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	require.NoError(t, db.AutoMigrate(&models.QueuedEmail{}))
	now := time.Date(2026, time.November, 19, 13, 38, 0, 0, time.FixedZone("EST", -5*60*60))
	for _, job := range []models.QueuedEmail{
		{Subject: "Due UTC", ScheduledFor: now.UTC().Add(-2 * time.Minute)},
		{Subject: "Due local", ScheduledFor: now.Add(-time.Minute)},
		{Subject: "Future local", ScheduledFor: now.In(time.FixedZone("PST", -8*60*60)).Add(time.Minute)},
	} {
		job.Recipient, job.TenantID = "recipient@example.test", "test"
		require.NoError(t, db.Create(&job).Error)
	}
	sender := &notificationRecorder{}
	queue := NewDBEmailQueue(db, sender, time.Hour)
	queue.Now = func() time.Time { return now }
	require.NoError(t, queue.ProcessBatch())
	require.Len(t, sender.sent, 2)
	require.Equal(t, "Due UTC", sender.sent[0].Subject)
	require.Equal(t, "Due local", sender.sent[1].Subject)
	pending := queue.Pending("test", "")
	require.Len(t, pending, 1)
	require.Equal(t, "Future local", pending[0].Subject)
}

func TestDBEmailQueueStoresExplicitScheduleInUTC(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/queue.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.QueuedEmail{}))
	queue := NewDBEmailQueue(db, nil, time.Hour)
	schedule := time.Now().In(time.FixedZone("east", 5*3600+30*60)).Add(time.Hour)
	item, err := queue.Queue(Email{TenantID: "test", To: "recipient@example.test"}, &schedule)
	require.NoError(t, err)
	var raw string
	require.NoError(t, db.Model(&models.QueuedEmail{}).Select("CAST(scheduled_for AS TEXT)").Where("id = ?", item.ID).Scan(&raw).Error)
	require.True(t, item.ScheduledFor.Equal(schedule))
	require.Equal(t, schedule.UTC().Format("2006-01-02 15:04:05.999999999-07:00"), raw)
}
