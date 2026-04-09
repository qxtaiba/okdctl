// Package cli wires openshitctl's cobra command tree (deploy, destroy,
// update-ingress, wizard) and shared CLI helpers for prompts, summaries,
// and config loading.
package cli

import (
	"bufio"
	"context"
	"os"
	"strings"
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
	_, _ = os.Stdout.WriteString(prompt)

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
