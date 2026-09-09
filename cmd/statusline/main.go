// Command statusline renders the Claude Code status line.
//
// It reads the status line JSON payload on stdin and writes two lines: where you
// are (repository, branch, HEAD) and what the session costs (model, context,
// money, rate limits). All the work happens in internal/statusline; this file
// only owns the process boundary — arguments, stdin, stdout, and the version
// stamp.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/AbdallaM-Ibrahim/claude-code-statusline/internal/statusline"
)

// version is stamped at build time with
// -ldflags "-X main.version=v1.2.3". An unstamped build says "dev", which is
// the honest answer for one built from a working tree.
var version = "dev"

// versionRequest reports whether argv asks for the version rather than a render.
//
// Claude Code invokes this with no arguments and a payload on stdin, so argument
// handling exists only so a downloaded binary can identify itself. Anything else
// is ignored rather than rejected: a status line that refuses to render because
// it did not recognise a flag is worse than one that ignores the flag.
func versionRequest(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[1] {
	case "--version", "-version", "version", "-V":
		return true
	}
	return false
}

func main() {
	if versionRequest(os.Args) {
		fmt.Println(version)
		return
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Println(statusline.Fallback)
		return
	}

	out := statusline.Render(raw)

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	fmt.Fprintln(w, out)
}
