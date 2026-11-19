package links

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
)

func DocumentRequestToken(signingKey, tenantID, requestID string) string {
	mac := hmac.New(sha256.New, []byte(signingKey))
	_, _ = io.WriteString(mac, "document-request:v1:"+tenantID+":"+requestID)
	return requestID + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
