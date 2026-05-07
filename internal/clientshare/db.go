package clientshare

import (
	"github.com/pixelcop/clientshare/internal/models"
	"github.com/pixelcop/clientshare/internal/tenant"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func bootstrapAdminUser(db *gorm.DB, cfg *Config) error {

	// Admin bootstrap logic
	adminEmail := cfg.Admin.BootstrapEmail
	adminPassword := cfg.Admin.BootstrapPassword
	if adminEmail == "" || adminPassword == "" {
		return nil
	}

	var count int64
	db.Model(&models.User{}).Where("tenant_id = ? AND role = ?", tenant.DefaultTenantID, "admin").Count(&count)
	if count > 0 {
		// already have an admin user
		return nil
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		zap.L().Fatal("failed to hash admin password", zap.Error(err))
		return err
	}
	admin := models.User{
		TenantID:     tenant.DefaultTenantID,
		Email:        adminEmail,
		PasswordHash: string(passwordHash),
		Role:         "admin",
		Name:         "Admin",
	}
	if err := db.Create(&admin).Error; err != nil {
		zap.L().Fatal("failed to create admin user", zap.Error(err))
		return err
	}
	zap.L().Info("Admin user bootstrapped", zap.String("email", adminEmail))
	return nil
}
