package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// PlanAction is the change verb okdctl gates a targeted plan on. It collapses
// terraform's raw resource_changes[].change.actions array into one value so
// node-lifecycle callers can assert intent (a delete, an in-place update)
// without re-implementing the create/delete → replace folding.
type PlanAction string

// Folded terraform plan actions. Noop is ["no-op"]/["read"]; Create/Update/
// Delete map the single-verb arrays; Replace is ["delete","create"] (or the
// reverse) — the destroy-and-recreate a resize gate must reject because it
// wipes the VM instead of mutating it in place; Unknown is anything else.
const (
	PlanActionNoop    PlanAction = "no-op"
	PlanActionCreate  PlanAction = "create"
	PlanActionUpdate  PlanAction = "update"
	PlanActionDelete  PlanAction = "delete"
	PlanActionReplace PlanAction = "replace"
	PlanActionUnknown PlanAction = "unknown"
)

// ResourceChange is the projected shape of one terraform-plan resource_changes
// entry: the address and its folded action.
type ResourceChange struct {
	Address string
	Action  PlanAction
}

// planShow is the subset of `terraform show -json <plan>` this package parses.
type planShow struct {
	ResourceChanges []struct {
		Address string `json:"address"`
		Change  struct {
			Actions []string `json:"actions"`
		} `json:"change"`
	} `json:"resource_changes"`
}

// foldActions collapses terraform's actions array into a single PlanAction.
func foldActions(actions []string) PlanAction {
	switch len(actions) {
	case 1:
		switch actions[0] {
		case "no-op":
			return PlanActionNoop
		case "create":
			return PlanActionCreate
		case "update":
			return PlanActionUpdate
		case "delete":
			return PlanActionDelete
		case "read":
			return PlanActionNoop
		}
	case 2:
		if slices.Contains(actions, "create") && slices.Contains(actions, "delete") {
			return PlanActionReplace
		}
	}
	return PlanActionUnknown
}

// ParsePlanChanges decodes `terraform show -json` output into the non-no-op
// resource changes. no-op entries are dropped so callers reason only about
// what the plan actually mutates.
func ParsePlanChanges(raw []byte) ([]ResourceChange, error) {
	var ps planShow
	if err := json.Unmarshal(raw, &ps); err != nil {
		return nil, fmt.Errorf("parse plan json: %w", err)
	}
	var out []ResourceChange
	for _, rc := range ps.ResourceChanges {
		action := foldActions(rc.Change.Actions)
		if action == PlanActionNoop {
			continue
		}
		out = append(out, ResourceChange{Address: rc.Address, Action: action})
	}
	return out, nil
}

// AssertOnlyChange verifies the plan's sole effective change is addr performing
// want. It is the safety gate node-lifecycle ops run before applying a targeted
// plan: an empty plan (variable never reached the module), an unexpected extra
// resource, a wrong address, or a wrong action (notably a replace where an
// update was intended) all return an error naming the offending set.
func AssertOnlyChange(changes []ResourceChange, addr string, want PlanAction) error {
	if len(changes) == 0 {
		return fmt.Errorf("plan gate: expected %s of %q but plan is empty (the variable may not reach the module)", want, addr)
	}
	if len(changes) != 1 {
		return fmt.Errorf("plan gate: expected exactly one change (%s of %q) but plan has %d: %s", want, addr, len(changes), describeChanges(changes))
	}
	got := changes[0]
	if got.Address != addr || got.Action != want {
		return fmt.Errorf("plan gate: expected %s of %q but plan has %s of %q", want, addr, got.Action, got.Address)
	}
	return nil
}

func describeChanges(changes []ResourceChange) string {
	parts := make([]string, len(changes))
	for i, c := range changes {
		parts[i] = fmt.Sprintf("%s %s", c.Action, c.Address)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// ShowPlanChanges runs `terraform show -json <planFile>` and returns the folded
// non-no-op resource changes. planFile is a saved plan produced by Plan with
// OutputPlanFile set.
func (t *Executor) ShowPlanChanges(ctx context.Context, planFile string) ([]ResourceChange, error) {
	result, err := t.exec.RunOutputChecked(ctx, 0, "terraform", "show", "-json", planFile)
	if err != nil {
		return nil, fmt.Errorf("terraform show plan: %w", err)
	}
	if result.Truncated {
		return nil, fmt.Errorf("terraform show plan: output truncated after %d bytes", len(result.Stdout))
	}
	return ParsePlanChanges([]byte(result.Stdout))
}
