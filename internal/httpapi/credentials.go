package httpapi

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
	"golang.org/x/crypto/bcrypt"
)

// Credentials is the parsed bcrypt-hash table loaded from credentials.yaml.
// Values are bcrypt hashes (cost >= 12 enforced at load time per AUTH-01).
// The zero value is an empty table — no users, all write requests return
// 401 (but authBasic still runs bcrypt against dummyHash to preserve
// timing-safety per AUTH-04).
type Credentials struct {
	users map[string][]byte
}

// credentialsFile is the on-disk YAML shape.
type credentialsFile struct {
	Users map[string]string `yaml:"users"`
}

// LoadCredentials reads credentials.yaml and returns a Credentials table.
// Returns an error if:
//   - the file is missing (operator explicitly configured credentials_file
//     in config.yaml — a missing file signals a misconfig, NOT an empty
//     registry; fail fast so the operator sees the problem at startup)
//   - the YAML is malformed
//   - any bcrypt hash has cost strictly less than 12 (AUTH-01)
func LoadCredentials(path string) (Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: read %q: %w", path, err)
	}
	var raw credentialsFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Credentials{}, fmt.Errorf("credentials: parse %q: %w", path, err)
	}
	users := make(map[string][]byte, len(raw.Users))
	for username, hashStr := range raw.Users {
		if username == "" {
			return Credentials{}, fmt.Errorf("credentials: %q: empty username", path)
		}
		hash := []byte(hashStr)
		cost, err := bcrypt.Cost(hash)
		if err != nil {
			return Credentials{}, fmt.Errorf("credentials: user %q: invalid bcrypt hash: %w", username, err)
		}
		if cost < 12 {
			return Credentials{}, fmt.Errorf("credentials: user %q: bcrypt cost %d is below minimum 12", username, cost)
		}
		users[username] = hash
	}
	return Credentials{users: users}, nil
}

// Lookup returns the bcrypt hash for a username. The second return is
// false if the user does not exist. Callers in authBasic MUST still run
// bcrypt.CompareHashAndPassword on a dummyHash fallback to preserve
// timing-safety (AUTH-04). This method is ONLY the lookup — it does NOT
// early-return, does NOT short-circuit, does NOT hash.
func (c Credentials) Lookup(username string) ([]byte, bool) {
	h, ok := c.users[username]
	return h, ok
}
