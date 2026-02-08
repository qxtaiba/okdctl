// Package cleanup provides cleanup utilities for OKD cluster artifacts.
// It handles removal of work directories, web server files, HAProxy configuration,
// dnsmasq settings, and Terraform state.
package cleanup

import (
	"context"
	"errors"
	"fmt"

	"github.com/qxtaiba/okd-proxmox-cli/internal/logging"
)

// Type defines the scope of cleanup operations.
type Type string

const (
	TypeFull          Type = "full"
	TypeWorkOnly      Type = "work-only"
	TypeWebOnly       Type = "web-only"
	TypeHAProxyOnly   Type = "haproxy-only"
	TypeTerraformOnly Type = "terraform-only"
)

// Options configures cleanup operations.
type Options struct {
	// Type determines what to clean up.
	Type Type

	// WorkDir is the working directory containing cluster artifacts.
	WorkDir string

	// TerraformEnv is the terraform environment to clean (optional).
	TerraformEnv string

	// ProjectRoot is the root of the project.
	ProjectRoot string

	// PreserveConfig keeps cluster configuration files when true.
	PreserveConfig bool

	// HTTPServerRoot is the web server root directory.
	HTTPServerRoot string

	// HAProxyConfig is the path to the haproxy configuration file.
	HAProxyConfig string

	// ClusterName is the name of the cluster (used for dnsmasq cleanup).
	ClusterName string

	// RemovePackages removes system packages installed during setup.
	// When true, packages like haproxy, httpd, dnsmasq, etc. will be uninstalled.
	RemovePackages bool

	// Logger for output messages.
	Logger logging.Logger
}

// getLogger returns the logger from options or a no-op logger if nil.
func (opts Options) getLogger() logging.Logger {
	if opts.Logger != nil {
		return opts.Logger
	}
	return logging.NoopLogger()
}

// Execute performs cleanup based on the provided options.
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
