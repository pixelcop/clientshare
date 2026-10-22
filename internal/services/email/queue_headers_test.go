package email

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type headerSender struct{ messages []Email }

func (s *headerSender) Send(email Email) error { s.messages = append(s.messages, email); return nil }

func TestQueuePreservesHeadersAndSeparatesPersonalMail(t *testing.T) {
	for _, mode := range []string{"memory", "database"} {
		t.Run(mode, func(t *testing.T) {
			sender := &headerSender{}
			var queue EmailQueue
			var database *gorm.DB
			if mode == "database" {
				var err error
				database, err = gorm.Open(sqlite.Open(t.TempDir()+"/queue.db"), &gorm.Config{})
				require.NoError(t, err)
				require.NoError(t, database.AutoMigrate(&models.QueuedEmail{}))
				queue = NewDBEmailQueue(database, sender, time.Hour)
			} else {
				queue = NewInMemoryEmailQueue(sender, time.Hour)
			}
			client := "client"
			later := time.Now().Add(time.Hour)
			// Personal messages for the same client must not be merged into a group.
			for _, to := range []string{"one@example.com", "two@example.com"} {
				_, err := queue.Queue(Email{TenantID: "tenant", ClientID: &client, To: to, ReplyTo: "requester@example.com", Subject: "Personal"}, &later)
				require.NoError(t, err)
			}
			// Private system notifications can batch when reply routing matches.
			for _, to := range []string{"one@example.com", "two@example.com"} {
				_, err := queue.Queue(Email{TenantID: "tenant", ClientID: &client, To: to, HideRecipients: true, ReplyTo: "operator@example.com", Subject: "Upload"}, &later)
				require.NoError(t, err)
			}
			_, err := queue.Queue(Email{TenantID: "tenant", ClientID: &client, To: "three@example.com", HideRecipients: true, ReplyTo: "other@example.com", Subject: "Different reply routing"}, &later)
			require.NoError(t, err)
			require.Len(t, queue.Pending("tenant", client), 4)
			if database != nil {
				queue = NewDBEmailQueue(database, sender, time.Hour)
			}
			require.NoError(t, queue.ProcessAll())
			require.Len(t, sender.messages, 4)
			for _, message := range sender.messages {
				require.Equal(t, "tenant", message.TenantID)
				if message.Subject == "Personal" {
					require.False(t, message.HideRecipients)
					require.Len(t, splitRecipients(message.To), 1)
					require.Equal(t, "requester@example.com", message.ReplyTo)
				} else if message.Subject == "Upload" {
					require.True(t, message.HideRecipients)
					require.Len(t, splitRecipients(message.To), 2)
					require.Equal(t, "operator@example.com", message.ReplyTo)
				} else {
					require.Equal(t, "other@example.com", message.ReplyTo)
				}
			}
		})
	}
}
