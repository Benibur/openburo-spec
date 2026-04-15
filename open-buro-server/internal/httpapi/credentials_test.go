package httpapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestLoadCredentials_Valid(t *testing.T) {
	creds, err := LoadCredentials("testdata/credentials-valid.yaml")
	require.NoError(t, err)
	hash, ok := creds.Lookup("admin")
	require.True(t, ok)
	cost, err := bcrypt.Cost(hash)
	require.NoError(t, err)
	require.Equal(t, 12, cost)
}

func TestLoadCredentials_Missing(t *testing.T) {
	_, err := LoadCredentials("testdata/does-not-exist.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "credentials")
}

func TestLoadCredentials_Malformed(t *testing.T) {
	_, err := LoadCredentials("testdata/credentials-malformed.yaml")
	require.Error(t, err)
}

func TestLoadCredentials_LowCost(t *testing.T) {
	_, err := LoadCredentials("testdata/credentials-low-cost.yaml")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cost")
	require.Contains(t, err.Error(), "12")
	require.Contains(t, err.Error(), "admin")
}
