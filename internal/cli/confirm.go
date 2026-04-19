package cli

import (
	"bufio"
	"context"
	"os"
	"strings"

	"golang.org/x/term"
)

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
	if !term.IsTerminal(int(os.Stdin.Fd())) { //nolint:gosec // G115: Fd() returns uintptr; always fits in int on supported platforms
		return false, nil
	}
	_, _ = os.Stderr.WriteString(prompt)

	inputCh := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
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
