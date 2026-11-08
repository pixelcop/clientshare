package email

import (
	"testing"
	"time"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestInMemoryQueueCannotBypassResourceDispatchValidation(t *testing.T) {
	sender := &notificationRecorder{}
	queue := NewInMemoryEmailQueue(sender, time.Hour)
	id := "bound-resource"
	for _, message := range []Email{{SecureLinkID: &id}, {AccessInvitationID: &id}, {IdentityInviteID: &id}, {RecipientUserID: &id}, {RecipientLinkID: &id}, {FileIDsCSV: id}} {
		_, err := queue.Queue(message, nil)
		require.ErrorIs(t, err, ErrAccessValidationUnavailable)
	}
	require.Empty(t, queue.Emails)
	queue.Emails = append(queue.Emails, models.QueuedEmail{ID: "unsafe", TenantID: "tenant", Recipient: "recipient@example.test", AccessInvitationID: &id})
	require.ErrorIs(t, queue.SendNow("tenant", "unsafe"), ErrAccessValidationUnavailable)
	require.NotNil(t, queue.Emails[0].CanceledAt)
	require.Empty(t, sender.sent)
}
