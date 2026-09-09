//go:build darwin

package usage

import (
	"context"
	"os/exec"
	"time"
)

// keychainTimeout bounds the one subprocess this program ever spawns.
const keychainTimeout = 800 * time.Millisecond

// keychainCredentials asks the login keychain for the record Claude Code keeps
// there on macOS instead of a credentials file. This is the only subprocess in
// the program: darwin only, only when the file is absent, only with the switch
// on. The output is the same JSON document the file would hold.
func keychainCredentials() []byte {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/security",
		"find-generic-password", "-s", "Claude Code-credentials", "-w").Output()
	if err != nil || len(out) == 0 || len(out) > maxCredentialBytes {
		return nil
	}
	return out
}
