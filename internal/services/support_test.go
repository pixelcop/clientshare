package services

import (
	"context"
	"errors"
	"github.com/pixelcop/clientshare/internal/models"
	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

type supportTestSender struct {
	fail bool
	sent []emailpkg.Email
}

func (s *supportTestSender) Send(e emailpkg.Email) error {
	if s.fail {
		return errors.New("offline")
	}
	s.sent = append(s.sent, e)
	return nil
}

func TestSupportDurabilityAndNotificationRetry(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/support.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&models.SupportMessage{}); err != nil {
		t.Fatal(err)
	}
	svc := SupportService{DB: db}
	row := models.SupportMessage{TenantID: "tenant", RequesterID: "user", RetryKey: "retry", Message: "Hello", Email: "reply@example.invalid", Destination: "support@example.invalid"}
	if err = svc.Receive(context.Background(), &row); err != nil {
		t.Fatal(err)
	}
	// Same retry key in another tenant must create its own receipt.
	other := row
	other.ID = ""
	other.TenantID = "other"
	if err = svc.Receive(context.Background(), &other); err != nil || other.ID == row.ID {
		t.Fatal(err)
	}
	sender := &supportTestSender{fail: true}
	if err = svc.Notify(sender); err != nil {
		t.Fatal(err)
	}
	db.First(&row, "id = ?", row.ID)
	if row.NotifiedAt != nil || row.Attempts != 1 {
		t.Fatal(row)
	}
	sender.fail = false
	if err = svc.Notify(sender); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 0 {
		t.Fatal("lease ignored")
	}
	db.Model(&models.SupportMessage{}).Where("1 = 1").Update("next_attempt_at", time.Now().Add(-time.Minute))
	if err = svc.Notify(sender); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 2 {
		t.Fatal("retry not delivered")
	}
	for _, message := range sender.sent {
		if message.To != "support@example.invalid" || message.ReplyTo != "reply@example.invalid" || message.HideRecipients {
			t.Fatalf("unexpected support headers: %+v", message)
		}
	}
	if err = svc.Notify(sender); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 2 {
		t.Fatal("duplicate notification")
	}
	db.First(&row, "id = ?", row.ID)
	if row.NotifiedAt == nil || row.Attempts != 2 {
		t.Fatal(row)
	}
}
