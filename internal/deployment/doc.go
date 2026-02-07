// Package deployment provides deployment orchestration for Kubernetes clusters.
//
// This package contains the core deployment logic separated from CLI concerns:
//
//   - Executor: Orchestrates deployment phases (prepare, install, configure)
//   - InterruptHandler: Manages graceful shutdown on interrupt signals
//   - Workflow: High-level deployment execution with options
//
// The CLI layer uses this package to implement deployment commands while
// keeping I/O and user interaction concerns separate from deployment logic.
package deployment
