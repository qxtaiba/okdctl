package cli

import (
	"bufio"
	"context"
	"os"
	"strings"

	"github.com/qxtaiba/okd-proxmox-cli/internal/tui"
)

// The stdin-reading goroutine cannot be cleanly cancelled but terminates when
// the process exits.
func promptForConfirmation(ctx context.Context, prompt string) (bool, error) {
	tui.Warn("this will create infrastructure resources")
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
