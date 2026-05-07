package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pixelcop/clientshare/internal/clientshare"
	"github.com/pixelcop/clientshare/internal/services"
	"github.com/pixelcop/clientshare/internal/tenant"
	"github.com/pixelcop/clientshare/pkg/db"
	"github.com/pixelcop/clientshare/pkg/utils"
	"go.uber.org/zap"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "Path to config file")
	csvPath := flag.String("csv", "", "Path to CSV file to import")
	invitedBy := flag.String("invited-by", "cli-import", "Value stored on invite_tokens.invited_by for newly created users")
	sendNow := flag.Bool("send-now", true, "Send newly queued invite emails immediately")
	flag.Parse()

	if *csvPath == "" {
		fmt.Fprintln(os.Stderr, "-csv is required")
		os.Exit(2)
	}

	utils.InitLogger()
	defer zap.L().Sync()

	cfg, err := clientshare.LoadConfig(*configPath)
	if err != nil {
		zap.S().Fatalf("Error loading config: %v", err)
	}

	store, err := clientshare.BuildStorage(context.Background(), cfg)
	if err != nil {
		zap.S().Fatalf("Error building storage: %v", err)
	}

	db, err := db.InitDB(cfg.Database, zap.L())
	if err != nil {
		zap.L().Fatal("database initialization failed", zap.Error(err))
	}
	emailQueue, err := clientshare.InitEmail(cfg, zap.L(), db)
	if err != nil {
		zap.L().Fatal("email initialization failed", zap.Error(err))
	}

	inviteTTL := 72 * time.Hour
	if cfg.Auth.InviteTokenDuration != "" {
		if parsed, err := time.ParseDuration(cfg.Auth.InviteTokenDuration); err == nil {
			inviteTTL = parsed
		} else {
			zap.L().Warn("invalid invite token duration, using default", zap.Error(err))
		}
	}

	file, err := os.Open(*csvPath)
	if err != nil {
		zap.L().Fatal("failed to open csv file", zap.Error(err), zap.String("path", *csvPath))
	}
	defer file.Close()

	importService := &services.ClientImportService{
		DB:             db,
		Storage:        store,
		EmailQueue:     emailQueue,
		TenantID:       tenant.DefaultTenantID,
		BaseURL:        cfg.Server.BaseURL,
		TokenSecret:    cfg.Auth.InviteTokenSecret,
		InviteTTL:      inviteTTL,
		TenantSettings: services.NewTenantSettingsService(db),
	}

	result, err := importService.ImportCSV(file, services.ClientImportOptions{InvitedBy: *invitedBy})
	if err != nil {
		zap.L().Fatal("client import failed", zap.Error(err))
	}

	if *sendNow {
		for _, inviteEmailID := range result.InviteEmailIDs {
			if err := emailQueue.SendNow(tenant.DefaultTenantID, inviteEmailID); err != nil {
				zap.L().Fatal("failed to send invite email", zap.Error(err), zap.String("queued_email_id", inviteEmailID))
			}
		}
	}

	fmt.Printf("Imported %d rows\n", result.RowsRead)
	fmt.Printf("Clients: %d created, %d existing\n", result.CreatedClients, result.ExistingClients)
	fmt.Printf("Users: %d created, %d updated\n", result.CreatedUsers, result.UpdatedUsers)
	if *sendNow {
		fmt.Printf("Invites sent: %d\n", result.InvitedUsers)
	} else {
		fmt.Printf("Invites queued: %d\n", result.InvitedUsers)
	}
}
