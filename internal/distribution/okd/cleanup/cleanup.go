// Package cleanup provides cleanup utilities for OKD cluster artifacts.
// It handles removal of work directories, web server files, HAProxy configuration,
// dnsmasq settings, and Terraform state.
package cleanup

import (
	"context"
	"errors"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/utils"
)

type Type string

const (
	TypeFull          Type = "full"
	TypeWorkOnly      Type = "work-only"
	TypeWebOnly       Type = "web-only"
	TypeHAProxyOnly   Type = "haproxy-only"
	TypeTerraformOnly Type = "terraform-only"
)

type Options struct {
	Type           Type
	WorkDir        string
	TerraformEnv   string
	ProjectRoot    string
	PreserveConfig bool
	HTTPServerRoot string
	HAProxyConfig  string
	ClusterName    string
	RemovePackages bool
	Logger         utils.Logger
}

func (opts Options) getLogger() utils.Logger {
	if opts.Logger != nil {
		return opts.Logger
	}
	return utils.NoopLogger()
}

func Execute(ctx context.Context, opts Options) error {
	logger := opts.getLogger()
	var errs []error

	switch opts.Type {
	case TypeFull:
		if err := WorkDirectory(ctx, opts.WorkDir, opts.PreserveConfig, logger); err != nil {
			errs = append(errs, err)
		}
		if err := WebServer(ctx, opts.HTTPServerRoot, logger); err != nil {
			errs = append(errs, err)
		}
		if err := HAProxy(ctx, opts.HAProxyConfig, logger); err != nil {
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

	case TypeWorkOnly:
		if err := WorkDirectory(ctx, opts.WorkDir, opts.PreserveConfig, logger); err != nil {
			errs = append(errs, err)
		}

	case TypeWebOnly:
		if err := WebServer(ctx, opts.HTTPServerRoot, logger); err != nil {
			errs = append(errs, err)
		}

	case TypeHAProxyOnly:
		if err := HAProxy(ctx, opts.HAProxyConfig, logger); err != nil {
			errs = append(errs, err)
		}

	case TypeTerraformOnly:
		if err := Terraform(ctx, opts.ProjectRoot, opts.TerraformEnv, logger); err != nil {
			errs = append(errs, err)
		}

	default:
		return fmt.Errorf("unknown cleanup type: %s (valid types: full, work-only, web-only, haproxy-only, terraform-only)", opts.Type)
	}

	printSummary(opts, logger)

	return errors.Join(errs...)
}
