//go:build !linux

package cli

import "context"

func collectDoctorOutput(_ context.Context) ([]byte, error) {
	return nil, nil
}
