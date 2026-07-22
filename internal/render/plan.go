package render

import (
	"fmt"

	"github.com/qxtaiba/okdctl/internal/infrastructure/terraform"
	"github.com/qxtaiba/okdctl/internal/tui"
)

// PlanPreview renders the drift-preview box shared by `okdctl plan` and
// `deploy --dry-run`: a clean confirmation when changes is empty, or a
// per-resource create/update/replace/delete listing when terraform found
// pending changes.
func PlanPreview(changes []terraform.ResourceChange) string {
	sb := NewBuilder()
	sb.WriteString("\n")

	if len(changes) == 0 {
		sb.WriteString("  " + tui.CompletionSuccess("no drift — infrastructure matches configuration") + "\n")
		sb.Newline()
		return "\n" + tui.BoxedSectionCompact(sb.String(), "plan preview", tui.DefaultBoxWidth) + "\n"
	}

	sb.WriteString("  " + tui.WarningStyle.Render(fmt.Sprintf("drift detected — %d pending change(s)", len(changes))) + "\n")
	sb.Newline()

	sb.Section("changes")
	for _, c := range changes {
		sb.KV(string(c.Action), c.Address)
	}
	sb.Newline()

	return "\n" + tui.BoxedSectionCompact(sb.String(), "plan preview", tui.DefaultBoxWidth) + "\n"
}
