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

// testStdinReader replaces os.Stdin for tests and bypasses the TTY guard; nil in production.
var testStdinReader io.Reader

// promptForLine reads a line from stdin with context awareness. Go can't
// cancel an in-flight read, so on ctx cancel the reader goroutine leaks
// bounded by the parent process's lifetime.
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

// promptForConfirmation reads a y/N answer from stdin; see promptForLine for the ctx/TTY contract.
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
// cluster name (case-sensitive); a typo or bare "y" both deny.
func promptForClusterNameConfirmation(ctx context.Context, name, prompt string) (bool, error) {
	response, err := promptForLine(ctx, prompt)
	if err != nil {
		return false, err
	}
	return response == name, nil
}

// confirmClusterMatches enforces the --yes/--confirm-cluster pairing used by
// destructive commands, returning *errtypes.UsageError (exit 64) when violated.
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
