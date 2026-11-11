package services

import "gorm.io/gorm"

// LockTenantAccess serializes authorization mutations, not reads. A role edit or
// group removal must not race an assignment whose ceiling was read before that
// revocation. PostgreSQL otherwise permits those disjoint row updates to both
// commit. SQLite already serializes writers and rejects a stale read-to-write
// upgrade. The lock is transaction-scoped and never held across network calls.
func LockTenantAccess(tx *gorm.DB, tenantID string) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "clientshare-access:"+tenantID).Error
}
