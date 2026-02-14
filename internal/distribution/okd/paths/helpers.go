package paths

import (
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

// WarnOnError returns an OnError callback that logs a warning with the given
// message prefix followed by the error. Use with StepBuilder.OnError().
func WarnOnError(logger utils.Logger, msg string) func(error) {
	return func(err error) {
		logger.Warn(fmt.Sprintf("%s: %v", msg, err))
	}
}
