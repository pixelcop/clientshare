package services

import (
	"errors"
	"strings"
)

func requireTenantID(tenantID string) (string, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return "", errors.New("tenant id is required")
	}
	return tenantID, nil
}
