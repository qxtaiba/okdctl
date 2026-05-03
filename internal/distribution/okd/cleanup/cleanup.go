// Package cleanup provides utilities for removing OKD cluster artifacts.
// Cleanup is best-effort: a mid-run crash leaves workDir in a partially-removed
// state with no resume capability. Terraform state is removed last so destroy
// stays re-runnable as long as earlier steps have not corrupted it. A two-pass
// design with a .cleanup-plan.json checkpoint would convert the flow to
// declarative resume; that is not yet implemented.
package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/qxtaiba/okdctl/internal/distribution/okd/phase"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/logutil"
)

// Kind selects which cleanup steps run.
type Kind string

// Cleanup kinds. Full removes everything; the *Only kinds scope cleanup to
// a single subsystem.
const (
	Full          Kind = "full"
	WorkOnly      Kind = "work-only"
	WebOnly       Kind = "web-only"
	HAProxyOnly   Kind = "haproxy-only"
	TerraformOnly Kind = "terraform-only"
)

// Options configures a cleanup run.
type Options struct {
	phase.BaseOptions

	Kind           Kind
	PreserveConfig bool
	HTTPServerRoot string
	HAProxyConfig  string
	VIP            string
	ClusterName    string
	RemovePackages bool
	BinDir         string
	Logger         *slog.Logger
}

func (opts *Options) getLogger() *slog.Logger {
	return logutil.OrNop(opts.Logger)
}

// ErrKindNotSet is returned by Execute when opts.Kind is empty.
// Callers can test for it with errors.Is.
var ErrKindNotSet = errors.New("cleanup kind not set")

// Execute runs the cleanup steps selected by opts.Kind. Individual step
// failures are accumulated and returned as a joined error; a partial run
// still attempts the remaining steps.
func Execute(ctx context.Context, opts *Options) error {
	logger := opts.getLogger().With("phase", "cleanup")
	var errs []error

	switch opts.Kind {
	case Full:
		if err := WorkDirectory(ctx, opts.WorkDir, opts.PreserveConfig, logger); err != nil {
			errs = append(errs, err)
		}
		if err := WebServer(ctx, opts.HTTPServerRoot, logger); err != nil {
			errs = append(errs, err)
		}
		if err := HAProxy(ctx, opts.HAProxyConfig, opts.VIP, logger); err != nil {
			errs = append(errs, err)
		}
		if err := Apache(ctx, logger); err != nil {
			errs = append(errs, err)
		}
		if err := Dnsmasq(ctx, opts.ClusterName, logger); err != nil {
			errs = append(errs, err)
		}
		if err := Terraform(ctx, opts.ProjectRoot, opts.TerraformEnv, logger); err != nil {
			errs = append(errs, err)
		}
		if opts.RemovePackages {
			if err := Packages(ctx, opts.BinDir, logger); err != nil {
				errs = append(errs, err)
			}
		}

	case WorkOnly:
		if err := WorkDirectory(ctx, opts.WorkDir, opts.PreserveConfig, logger); err != nil {
			errs = append(errs, err)
		}

	case WebOnly:
		if err := WebServer(ctx, opts.HTTPServerRoot, logger); err != nil {
			errs = append(errs, err)
		}

	case HAProxyOnly:
		if err := HAProxy(ctx, opts.HAProxyConfig, opts.VIP, logger); err != nil {
			errs = append(errs, err)
		}

	case TerraformOnly:
		if err := Terraform(ctx, opts.ProjectRoot, opts.TerraformEnv, logger); err != nil {
			errs = append(errs, err)
		}

	default:
		if opts.Kind == "" {
			return &errtypes.ConfigError{Msg: "cleanup kind not set", Err: ErrKindNotSet}
		}
		return &errtypes.ConfigError{Msg: fmt.Sprintf("unknown cleanup type: %s (valid types: full, work-only, web-only, haproxy-only, terraform-only)", opts.Kind)}
	}

	printSummary(opts, logger)

	return errors.Join(errs...)
}
