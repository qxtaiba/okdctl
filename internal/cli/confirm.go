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
// promptForLine and bypasses the TTY guard. Tests set this; production code
// never touches it.
var testStdinReader io.Reader

// promptForLine reads a single line from stdin with context awareness.
//
// Design note: Go has no portable way to cancel an in-flight stdin read, so
// we start a single reader goroutine and race it against ctx.Done. On ctx
// cancel the function returns immediately; the reader goroutine remains
// blocked on Stdin.Read until either the user presses enter or the process
// exits. Because inputCh has capacity 1, the goroutine's eventual send never
// blocks — it simply writes into an abandoned channel and returns. This is
// a bounded leak scoped to the lifetime of the parent process, not a true
// resource leak.
func promptForLine(ctx context.Context, prompt string) (string, error) {
	r := testStdinReader
	if r == nil {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return "", &errtypes.UsageError{Msg: "no TTY and --yes not set; refusing destructive op"}
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
		return "", ctx.Err()
	case response := <-inputCh:
		return response, nil
	}
}

// promptForConfirmation reads a y/N answer from stdin. See promptForLine for
// the ctx-cancellation and TTY-guard contract.
func promptForConfirmation(ctx context.Context, prompt string) (bool, error) {
	response, err := promptForLine(ctx, prompt)
	if err != nil {
		return false, err
	}
	return isConfirmResponse(response), nil
}

func isConfirmResponse(response string) bool {
	return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes")
}

// promptForClusterNameConfirmation requires the operator to type the exact
// cluster name (case-sensitive) before a destructive op proceeds — a typo or
// a plain "y" both deny, unlike promptForConfirmation's y/N gate. See
// promptForLine for the ctx-cancellation and TTY-guard contract.
func promptForClusterNameConfirmation(ctx context.Context, name, prompt string) (bool, error) {
	response, err := promptForLine(ctx, prompt)
	if err != nil {
		return false, err
	}
	return response == name, nil
}

// confirmClusterMatches enforces the --yes / --confirm-cluster pairing used
// by destructive commands. When force is false the check is skipped (the
// interactive promptForConfirmation path handles that case). Returns
// *errtypes.UsageError when the guard is violated (the fix is to change the
// command line, so it maps to exit 64); nil otherwise.
func confirmClusterMatches(force bool, confirm, name, verb string) error {
	if !force {
		return nil
	}
	if confirm == "" {
		return &errtypes.UsageError{
			Msg: fmt.Sprintf("--yes requires --confirm-cluster=%q to guard against scripted %ss against the wrong cluster", name, verb),
		}
	}
	if confirm != name {
		return &errtypes.UsageError{
			Msg: fmt.Sprintf("--confirm-cluster %q does not match config cluster %q; refusing %s",
				confirm, name, verb),
		}
	}
	return nil
}
