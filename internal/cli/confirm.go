package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/errtypes"
)

// testStdinReader, when non-nil, replaces os.Stdin as the input source for
// promptForConfirmation and bypasses the TTY guard. Tests set this; production
// code never touches it.
var testStdinReader io.Reader

// promptForConfirmation reads a y/N answer from stdin with context awareness.
//
// Design note: Go has no portable way to cancel an in-flight stdin read, so
// we start a single reader goroutine and race it against ctx.Done. On ctx
// cancel the function returns immediately; the reader goroutine remains
// blocked on Stdin.Read until either the user presses enter or the process
// exits. Because inputCh has capacity 1, the goroutine's eventual send never
// blocks — it simply writes into an abandoned channel and returns. This is
// a bounded leak scoped to the lifetime of the parent process, not a true
// resource leak.
func promptForConfirmation(ctx context.Context, prompt string) (bool, error) {
	r := testStdinReader
	if r == nil {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return false, &errtypes.ConfigError{Msg: "no TTY and --yes not set; refusing destructive op"}
		}
		r = os.Stdin
	}
	_, _ = os.Stderr.WriteString(prompt)

	inputCh := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(r)
		line, err := reader.ReadString('\n')
		if err != nil {
			inputCh <- ""
			return
		}
		inputCh <- strings.TrimSpace(line)
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case response := <-inputCh:
		return isConfirmResponse(response), nil
	}
}

func isConfirmResponse(response string) bool {
	return response == "y" || response == "Y" || response == "yes"
}

// confirmClusterMatches enforces the --yes / --confirm-cluster pairing used
// by destructive commands. When force is false the check is skipped (the
// interactive promptForConfirmation path handles that case). Returns
// *errtypes.ConfigError when the guard is violated; nil otherwise.
func confirmClusterMatches(force bool, confirm, name, verb string) error {
	if !force {
		return nil
	}
	if confirm == "" {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("--yes requires --confirm-cluster=%q to guard against scripted %ss against the wrong cluster", name, verb),
		}
	}
	if confirm != name {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("--confirm-cluster %q does not match config cluster %q; refusing %s",
				confirm, name, verb),
		}
	}
	return nil
}
