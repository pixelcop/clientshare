package clientshare

import (
	"time"

	emailpkg "github.com/pixelcop/clientshare/internal/services/email"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func InitEmail(cfg *Config, logger *zap.Logger, db *gorm.DB) (emailpkg.EmailQueue, error) {
	if !cfg.Email.Enabled {
		logger.Info("Email notifications disabled")
		return nil, nil
	}

	logger.Info("Email notifications enabled")
	batchDelayMin := cfg.Email.BatchDelayMinutes
	if batchDelayMin <= 0 {
		batchDelayMin = 15
	}
	batchDelay := time.Duration(batchDelayMin) * time.Minute

	sender := &emailpkg.SMTPSender{
		Host:     cfg.Email.SMTP.Host,
		Port:     cfg.Email.SMTP.Port,
		Username: cfg.Email.SMTP.Username,
		Password: cfg.Email.SMTP.Password,
		From:     cfg.Email.SMTP.From,
	}
	var emailSender emailpkg.EmailSender = sender
	if cfg.Email.SendRate > 0 {
		emailSender = emailpkg.NewRateLimitedSender(emailSender, cfg.Email.SendRate)
		logger.Info("Email sender rate limiting enabled", zap.Int("send_rate_per_second", cfg.Email.SendRate))
	}
	var queue emailpkg.EmailQueue
	if db != nil {
		queue = emailpkg.NewDBEmailQueue(db, emailSender, batchDelay)
	} else {
		queue = emailpkg.NewInMemoryEmailQueue(emailSender, batchDelay)
	}
	worker := emailpkg.NewWorker(queue, time.Minute)
	worker.Start()
	return queue, nil
}
