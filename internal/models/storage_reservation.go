package models

import "time"

// StorageReservation accounts for in-flight uploads across server processes.
type StorageReservation struct {
	ID        string    `gorm:"primaryKey;size:26"`
	TenantID  string    `gorm:"not null;index"`
	Bytes     int64     `gorm:"not null"`
	ExpiresAt time.Time `gorm:"not null;index"`
}
