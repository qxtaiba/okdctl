// Package cli wires openshitctl's cobra command tree (deploy, destroy,
// update-ingress, wizard) and shared CLI helpers for prompts, summaries,
// and viper-backed config loading.
package cli

import (
	"bufio"
	"context"
	"os"
	"strings"
)

// The stdin-reading goroutine cannot be cleanly cancelled but terminates when
// the process exits.
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
		return false, nil
	case response := <-inputCh:
		return isConfirmResponse(response), nil
	}
}

func isConfirmResponse(response string) bool {
	return response == "y" || response == "Y" || response == "yes"
}
