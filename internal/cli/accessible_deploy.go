package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/qxtaiba/okdctl/internal/config"
	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/tui/wizard"
	"github.com/qxtaiba/okdctl/internal/tui/wizard/steps"
	"github.com/qxtaiba/okdctl/internal/wizarddraft"
)

type accessiblePrompt interface {
	line(string, string) (string, error)
	secret(string) (string, error)
	writer() io.Writer
}

const accessibleEnv = "OKDCTL_ACCESSIBLE"

func accessibleRequested(flag bool, getenv func(string) string) bool {
	return flag || getenv(accessibleEnv) == "1"
}

type terminalAccessiblePrompt struct {
	in  io.Reader
	out io.Writer
	fd  int
	buf *bufio.Reader
}

func newTerminalAccessiblePrompt(in io.Reader, out io.Writer, fd int) *terminalAccessiblePrompt {
	return &terminalAccessiblePrompt{in: in, out: out, fd: fd, buf: bufio.NewReaderSize(oneByteReader{r: in}, 1)}
}

type oneByteReader struct{ r io.Reader }

func (r oneByteReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.r.Read(p)
}

func (p *terminalAccessiblePrompt) line(label, current string) (string, error) {
	if current == "" {
		fmt.Fprintf(p.out, "%s: ", label)
	} else {
		fmt.Fprintf(p.out, "%s [%s]: ", label, current)
	}
	value, err := p.buf.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if err != nil && value == "" {
		return "", io.EOF
	}
	return strings.TrimSuffix(strings.TrimSuffix(value, "\n"), "\r"), nil
}

func (p *terminalAccessiblePrompt) secret(label string) (string, error) {
	fmt.Fprintf(p.out, "%s (input hidden): ", label)
	value, err := term.ReadPassword(p.fd)
	fmt.Fprintln(p.out)
	return string(value), err
}

func (p *terminalAccessiblePrompt) writer() io.Writer { return p.out }

func runAccessibleDeployConfigure(cmd *cobra.Command, cfg *config.Config, configExists bool) (hubOutcome, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return hubOutcome{}, &errtypes.UsageError{Msg: "deploy --accessible needs a terminal for hidden secret input; use --write-config or edit the config file for non-interactive setup"}
	}
	prompt := newTerminalAccessiblePrompt(cmd.InOrStdin(), cmd.OutOrStdout(), int(os.Stdin.Fd()))
	draft, hasDraft := loadWizardDraft(wizardDraftPath(deployOutputFile))
	if hasDraft {
		cfg = draft.Config
	}
	saveDraft := func(cfg *config.Config, stepID wizard.StepID) error {
		return wizarddraft.New(wizardDraftPath(deployOutputFile)).Save(cfg, wizarddraft.Cursor{StepID: stepID}, time.Now())
	}
	return runAccessibleDeployConfigureWithDraft(cmd.Context(), prompt, cfg, configExists || hasDraft, saveDraft)
}

func runAccessibleDeployConfigureWith(ctx context.Context, prompt accessiblePrompt, cfg *config.Config, configExists bool) (hubOutcome, error) {
	return runAccessibleDeployConfigureWithDraft(ctx, prompt, cfg, configExists, nil)
}

func runAccessibleDeployConfigureWithDraft(ctx context.Context, prompt accessiblePrompt, cfg *config.Config, configExists bool, saveDraft func(*config.Config, wizard.StepID) error) (hubOutcome, error) {
	wizardCfg := wizard.DefaultConfig()
	wizardCfg.InitialConfig = cfg
	wizardCfg.ConfigExists = configExists
	built := buildWizardStepsWithState(wizardCfg)
	for _, step := range built.Steps {
		if err := ctx.Err(); err != nil {
			return accessibleCancelled(cfg)
		}
		if conditional, ok := step.(wizard.ConditionalStep); ok && !conditional.ShouldShow(cfg) {
			continue
		}
		switch s := step.(type) {
		case *steps.WelcomeStep, *steps.ReviewStep:
			continue
		case *steps.DistributionStep:
			current := cfg.Distribution.Version
			value, err := prompt.line("OKD version (for example 4.18)", current)
			if err != nil {
				return accessibleCancelledOrError(ctx, cfg, err)
			}
			if value != "" {
				s.SetSelectedVersion(value)
			}
			if err := s.Validate(); err != nil {
				return accessibleError(cfg, err)
			}
			if err := s.Apply(cfg); err != nil {
				return accessibleError(cfg, err)
			}
			if err := saveAccessibleDraft(saveDraft, cfg, step.ID()); err != nil {
				return accessibleError(cfg, err)
			}
		case *steps.NodePlacementStep:
			if cfg.Provider.Proxmox != nil {
				if err := promptProxmoxPlacement(ctx, prompt, cfg.Provider.Proxmox); err != nil {
					return accessibleCancelledOrError(ctx, cfg, err)
				}
			}
			if err := saveAccessibleDraft(saveDraft, cfg, step.ID()); err != nil {
				return accessibleError(cfg, err)
			}
		case *wizard.DataDrivenStep:
			for {
				if err := promptDataDrivenStep(ctx, prompt, s); err != nil {
					return accessibleCancelledOrError(ctx, cfg, err)
				}
				if err := s.Validate(); err != nil {
					fmt.Fprintf(prompt.writer(), "Validation error in %s: %v. Please review this step again.\n", step.Title(), err)
					continue
				}
				break
			}
			if err := s.Apply(cfg); err != nil {
				return accessibleError(cfg, fmt.Errorf("%s: %w", step.Title(), err))
			}
			if err := saveAccessibleDraft(saveDraft, cfg, step.ID()); err != nil {
				return accessibleError(cfg, err)
			}
		default:
			if applier, ok := step.(wizard.ConfigApplier); ok {
				if err := applier.Apply(cfg); err != nil {
					return accessibleError(cfg, fmt.Errorf("%s: %w", step.Title(), err))
				}
			}
		}
	}

	if result := cfg.Validate(); !result.IsValid() {
		return accessibleError(cfg, fmt.Errorf("configuration validation: %s", result.Error()))
	}
	writeAccessibleSummary(prompt.writer(), cfg)
	return chooseAccessibleAction(ctx, prompt, cfg)
}

func writeAccessibleSummary(out io.Writer, cfg *config.Config) {
	fmt.Fprintf(out, "\nConfiguration validated for %s.\n", cfg.Cluster.Name)
	fmt.Fprintf(out, "Domain: %s\n", cfg.Cluster.Domain)
	fmt.Fprintf(out, "OKD version: %s\n", cfg.Distribution.Version)
	if cfg.Provider.Proxmox != nil {
		fmt.Fprintf(out, "Proxmox host: %s\n", cfg.Provider.Proxmox.Host)
	}
	fmt.Fprintf(out, "Control plane nodes: %d; workers: %d.\n",
		cfg.Topology.ControlPlane.Count, cfg.Topology.Workers.Count)
	fmt.Fprintln(out, "Credentials are omitted from this transcript.")
	fmt.Fprintln(out, "Choose an action:")
}

func chooseAccessibleAction(ctx context.Context, prompt accessiblePrompt, cfg *config.Config) (hubOutcome, error) {
	choice, err := prompt.line("1 save configuration, 2 deploy, 3 cancel", "1")
	if err != nil {
		return accessibleCancelledOrError(ctx, cfg, err)
	}
	switch strings.TrimSpace(choice) {
	case "", "1":
		return hubOutcome{Result: wizard.Result{Completed: true, Config: cfg, Action: wizard.ActionExit}}, nil
	case "2":
		return hubOutcome{Result: wizard.Result{Completed: true, Config: cfg, Action: wizard.ActionDeploy}}, nil
	case "3":
		return accessibleCancelled(cfg)
	default:
		return accessibleError(cfg, &errtypes.UsageError{Msg: "choose 1, 2, or 3"})
	}
}

func accessibleCancelled(cfg *config.Config) (hubOutcome, error) {
	clearConfigCredentials(cfg)
	return hubOutcome{Result: wizard.Result{Cancelled: true}}, nil
}

func saveAccessibleDraft(save func(*config.Config, wizard.StepID) error, cfg *config.Config, stepID wizard.StepID) error {
	if save == nil {
		return nil
	}
	if err := save(cfg, stepID); err != nil {
		return fmt.Errorf("save wizard draft: %w", err)
	}
	return nil
}

func promptProxmoxPlacement(ctx context.Context, prompt accessiblePrompt, px *config.ProxmoxConfig) error {
	fmt.Fprintln(prompt.writer(), "\nProxmox placement (comma-separate node assignments; enter - to clear a value; names are not discovered live)")
	fields := []struct {
		label string
		value string
		set   func(string)
	}{
		{"bootstrap Proxmox node", px.Node, func(v string) { px.Node = v }},
		{"VM network bridge", px.Bridge, func(v string) { px.Bridge = v }},
		{"additional VM bridges (comma-separated; blank for none)", accessibleBridgeList(px.AdditionalNetworks), func(v string) {
			px.AdditionalNetworks = accessibleAdditionalNetworks(v, px.AdditionalNetworks)
		}},
		{"OS storage pool", px.Storage, func(v string) { px.Storage = v }},
		{"data storage pool", px.DataStorage, func(v string) { px.DataStorage = v }},
		{"ISO storage pool", px.ISOStorage, func(v string) { px.ISOStorage = v }},
		{"pre-uploaded FCOS ISO (blank for automatic download)", px.FCOSIso, func(v string) { px.FCOSIso = v }},
		{"control plane Proxmox nodes", strings.Join(px.ControlPlaneNodes, ","), func(v string) { px.ControlPlaneNodes = accessibleCSV(v) }},
		{"worker Proxmox nodes", strings.Join(px.WorkerNodes, ","), func(v string) { px.WorkerNodes = accessibleCSV(v) }},
	}
	for _, field := range fields {
		if err := ctx.Err(); err != nil {
			return err
		}
		value, err := prompt.line(field.label, field.value)
		if err != nil {
			return err
		}
		if value != "" {
			if value == "-" {
				value = ""
			}
			field.set(value)
		}
	}
	return nil
}

func accessibleBridgeList(networks []config.AdditionalNetwork) string {
	bridges := make([]string, len(networks))
	for i := range networks {
		bridges[i] = networks[i].Bridge
	}
	return strings.Join(bridges, ",")
}

func accessibleAdditionalNetworks(value string, current []config.AdditionalNetwork) []config.AdditionalNetwork {
	old := make(map[string]config.AdditionalNetwork, len(current))
	for _, network := range current {
		old[network.Bridge] = network
	}
	var result []config.AdditionalNetwork
	for _, bridge := range accessibleCSV(value) {
		network, ok := old[bridge]
		if !ok {
			network = config.AdditionalNetwork{Bridge: bridge, Model: "virtio"}
		}
		result = append(result, network)
	}
	return result
}

func accessibleCSV(value string) []string {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "-" || strings.EqualFold(strings.TrimSpace(value), "none") {
		return nil
	}
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func promptDataDrivenStep(ctx context.Context, prompt accessiblePrompt, step *wizard.DataDrivenStep) error {
	definition := step.Definition()
	fmt.Fprintf(prompt.writer(), "\n%s\n", definition.Title)
	allValues := make(map[string]string)
	for sectionIndex := range definition.Sections {
		section := &definition.Sections[sectionIndex]
		for fieldIndex := range section.Fields {
			field := &section.Fields[fieldIndex]
			allValues[field.Key] = step.Value(field.Key)
		}
	}
	for sectionIndex := range definition.Sections {
		section := &definition.Sections[sectionIndex]
		if section.Visible != nil && !section.Visible(allValues) {
			continue
		}
		fmt.Fprintf(prompt.writer(), "%s\n", section.Title)
		for fieldIndex := range section.Fields {
			field := &section.Fields[fieldIndex]
			if err := ctx.Err(); err != nil {
				return err
			}
			label := field.Label
			if field.Required {
				label += " (required)"
			}
			if field.Help != "" {
				fmt.Fprintf(prompt.writer(), "  %s\n", field.Help)
			}
			current := step.Value(field.Key)
			var value string
			var err error
			switch {
			case accessibleSecretField(field):
				value, err = prompt.secret(label)
			case len(field.Options) > 0:
				fmt.Fprintf(prompt.writer(), "  Options: %s\n", strings.Join(field.Options, ", "))
				value, err = prompt.line(label, current)
			default:
				value, err = prompt.line(label, current)
			}
			if err != nil {
				return err
			}
			if value != "" {
				if field.Validate != nil {
					if err := field.Validate(value); err != nil {
						if accessibleSecretField(field) {
							return fmt.Errorf("%s: secret value is invalid", field.Label)
						}
						return fmt.Errorf("%s: %w", field.Label, err)
					}
				}
				if !step.SetValue(field.Key, value) {
					return fmt.Errorf("unknown wizard field %q", field.Key)
				}
				allValues[field.Key] = value
			}
		}
	}
	return nil
}

func accessibleSecretField(field *wizard.FieldDefinition) bool {
	if field.Type == wizard.FieldTypePassword {
		return true
	}
	key := strings.ToLower(field.Key)
	return key == "token_id" || strings.HasSuffix(key, "_token_value") ||
		strings.HasSuffix(key, "_secret_value") || strings.HasSuffix(key, "_api_key") ||
		strings.HasSuffix(key, "_private_key")
}

func accessibleCancelledOrError(ctx context.Context, cfg *config.Config, err error) (hubOutcome, error) {
	if ctx.Err() != nil || errors.Is(err, io.EOF) {
		return accessibleCancelled(cfg)
	}
	return accessibleError(cfg, err)
}

func accessibleError(cfg *config.Config, err error) (hubOutcome, error) {
	clearConfigCredentials(cfg)
	return hubOutcome{}, err
}
