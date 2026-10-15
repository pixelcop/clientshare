package handlers

import (
	"encoding/json"
	"testing"

	"github.com/pixelcop/clientshare/internal/models"
	"github.com/stretchr/testify/require"
)

func TestPasskeyUserFromRecordsLegacyAttestationFormat(t *testing.T) {
	for _, format := range []string{"none", "packed", "fido-u2f"} {
		t.Run(format, func(t *testing.T) {
			// Literal pre-v0.17 storage shape, independent of the current encoder.
			data := []byte(`{"id":"Y3JlZGVudGlhbA==","publicKey":"cHVibGljLWtleQ==","attestationType":"` + format + `","transport":["internal"],"flags":{"userPresent":true,"userVerified":true,"backupEligible":true,"backupState":true},"authenticator":{"signCount":42}}`)
			user := models.User{ID: "user-id"}
			loaded, err := passkeyUserFromRecords(user, []models.PasskeyCredential{{CredentialData: data}})
			require.NoError(t, err)
			require.Len(t, loaded.credentials, 1)
			credential := loaded.credentials[0]
			require.Equal(t, []byte("credential"), credential.ID)
			require.Equal(t, []byte("public-key"), credential.PublicKey)
			require.Equal(t, format, credential.AttestationFormat)
			require.Empty(t, credential.AttestationType)
			require.EqualValues(t, 42, credential.Authenticator.SignCount)
			require.True(t, credential.Flags.UserVerified)
			require.True(t, credential.Flags.BackupEligible)
			require.True(t, credential.Flags.BackupState)
			require.Equal(t, format, credential.Descriptor().AttestationFormat)

			// Successful logins re-save the whole credential; ensure that round trip
			// preserves the migrated record and its authentication state.
			encoded, err := json.Marshal(credential)
			require.NoError(t, err)
			reloaded, err := passkeyUserFromRecords(user, []models.PasskeyCredential{{CredentialData: encoded}})
			require.NoError(t, err)
			require.Equal(t, loaded.credentials, reloaded.credentials)
		})
	}
}
