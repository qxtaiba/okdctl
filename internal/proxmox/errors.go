package proxmox

import "errors"

var (
	// ErrNotFound reports a node, VM, storage or volume the cluster does not have.
	// It names the request's subject, so only Status, VMConfig, VMPending and
	// DeleteVM may read it as "VM gone".
	ErrNotFound = errors.New("not found")
	// ErrPermission reports an API call the token lacks a privilege for.
	ErrPermission = errors.New("permission denied")
)

// TaskFailed reports a Proxmox task that ended with a non-OK exit status.
type TaskFailed struct {
	UPID       string
	ExitStatus string
}

func (e *TaskFailed) Error() string { return e.ExitStatus }
