package terraform

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// PlanAction folds terraform's raw resource_changes[].change.actions array into one gate-able verb.
type PlanAction string

// Replace (create+delete in either order) is a destroy-and-recreate; a resize
// gate must reject it, not treat it as in-place.
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

func foldActions(actions []string) PlanAction {
	switch len(actions) {
	case 1:
		switch actions[0] {
		case "no-op", "read":
			return PlanActionNoop
		case "create":
			return PlanActionCreate
		case "update":
			return PlanActionUpdate
		case "delete":
			return PlanActionDelete
		}
	case 2:
		if slices.Contains(actions, "create") && slices.Contains(actions, "delete") {
			return PlanActionReplace
		}
	}
	return PlanActionUnknown
}

// parsePlanChanges decodes terraform show -json output, dropping no-op entries
// so callers reason only about actual mutations.
func parsePlanChanges(raw []byte) ([]ResourceChange, error) {
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

// AssertOnlyChange verifies the plan's sole effective change is addr
// performing want; node-lifecycle ops run this before applying a targeted plan.
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

// EmptyPlanMeansAlreadyAtTarget interprets an empty targeted plan: delete
// means already-gone only if addr is absent from state; any other want means
// already-there only if addr is present — distinguishing a resumed re-run from
// a plan that never reached the module.
func EmptyPlanMeansAlreadyAtTarget(addrInState bool, want PlanAction) bool {
	if want == PlanActionDelete {
		return !addrInState
	}
	return addrInState
}

// ShowPlanChanges runs terraform show -json <planFile> and returns the folded
// non-no-op resource changes; planFile is a saved plan from Plan with OutputPlanFile set.
func (t *Executor) ShowPlanChanges(ctx context.Context, planFile string) ([]ResourceChange, error) {
	result, err := t.exec.RunOutputChecked(ctx, 0, "terraform", "show", "-json", planFile)
	if err != nil {
		return nil, fmt.Errorf("terraform show plan: %w", err)
	}
	if result.Truncated {
		return nil, fmt.Errorf("terraform show plan: output truncated after %d bytes", len(result.Stdout))
	}
	return parsePlanChanges([]byte(result.Stdout))
}
