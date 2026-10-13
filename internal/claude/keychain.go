package claude

import (
	"crypto/sha256"
	"encoding/hex"
)

// Keychain holds Claude's tokens on macOS. Claude reads its keychain item first and falls back
// to .credentials.json only when the item is missing, so a stale file there means nothing.
type Keychain interface {
	Get(service string) ([]byte, error) // nil, nil when the item does not exist
	Set(service string, data []byte) error
	Delete(service string) error // no error when the item does not exist
}

const keychainBase = "Claude Code-credentials"

// keychainService is the item Claude uses for a config dir given through CLAUDE_CONFIG_DIR:
// the base name plus the first 8 hex digits of the dir's SHA-256.
func keychainService(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	return keychainBase + "-" + hex.EncodeToString(sum[:])[:8]
}
