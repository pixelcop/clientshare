package models

import "github.com/pixelcop/clientshare/internal/tenant"

func ensureTenantID(tenantID *string) {
	if *tenantID == "" {
		*tenantID = tenant.DefaultTenantID
	}
}
