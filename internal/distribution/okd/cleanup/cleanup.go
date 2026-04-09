// Package cleanup provides cleanup utilities for OKD cluster artifacts.
// It handles removal of work directories, web server files, HAProxy configuration,
// dnsmasq settings, and Terraform state.
package cleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

type Kind string

const (
	Full          Kind = "full"
	WorkOnly      Kind = "work-only"
	WebOnly       Kind = "web-only"
	HAProxyOnly   Kind = "haproxy-only"
	TerraformOnly Kind = "terraform-only"
)

type Options struct {
	Kind           Kind
	WorkDir        string
	TerraformEnv   string
	ProjectRoot    string
	PreserveConfig bool
	HTTPServerRoot string
	HAProxyConfig  string
	VIP            string
	ClusterName    string
	RemovePackages bool
	Logger         *slog.Logger
}

func (opts *Options) getLogger() *slog.Logger {
	if opts.Logger != nil {
		return opts.Logger
	}
	return slog.New(slog.DiscardHandler)
}

func Execute(ctx context.Context, opts *Options) error {
	logger := opts.getLogger()
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
			if err := Packages(ctx, logger); err != nil {
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
		return fmt.Errorf("unknown cleanup type: %s (valid types: full, work-only, web-only, haproxy-only, terraform-only)", opts.Kind)
	}

	printSummary(opts, logger)

	return errors.Join(errs...)
}
