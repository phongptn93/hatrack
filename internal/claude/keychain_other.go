//go:build !darwin

package claude

// systemKeychain is nil off macOS: Claude keeps its tokens in .credentials.json there.
func systemKeychain() Keychain { return nil }
