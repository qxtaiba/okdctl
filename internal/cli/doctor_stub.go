//go:build !linux

package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func init() {
	doctorCmd.RunE = runDoctorStub
}

func runDoctorStub(_ *cobra.Command, _ []string) error {
	return fmt.Errorf("okdctl doctor is only supported on linux (current: %s)", runtime.GOOS)
}
