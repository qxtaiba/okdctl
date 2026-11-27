package proxmox

import "errors"

var (
	// ErrNotFound reports a node, VM, storage, volume or HA resource the
	// cluster does not have. It names the request's subject, so only Status,
	// VMConfig, VMPending, DeleteVM and SetHA's registration read may take it
	// as "VM gone"; every other reader treats it as a plain failure.
	ErrNotFound = errors.New("not found")
	// ErrPermission reports an API call the token lacks a privilege for.
	ErrPermission = errors.New("permission denied")
)

// TaskFailed reports a Proxmox task that ended with a non-OK exit status.
type TaskFailed struct {
	UPID       string
	ExitStatus string
}

func (e *TaskFailed) Error() string {
	if e.ExitStatus == "" {
		return "task " + e.UPID + " failed"
	}
	return e.ExitStatus
}
