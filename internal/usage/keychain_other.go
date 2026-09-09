//go:build !darwin

package usage

// keychainCredentials has nothing to ask on Windows and Linux: Claude Code keeps
// its credentials in a file there.
func keychainCredentials() []byte { return nil }
