# okdctl full audit — 2026-04-21

**Orchestrated by** `audit-all` across 14 specialized audit skills.


## Executive summary

The repo closed **116 findings** since 2026-04-20 against **60 new**, a net reduction of 56 items and a resolution ratio of 44%. The biggest remaining signal is **critical-path test gaps and exported-doc regressions** — `audit-tests` carries the only blocker (`no-test-kubeconfig-merge-full`) plus 13 majors, and `audit-documentation` surfaced 24 majors for exported symbols whose doc comments were stripped by a recent comment-hygiene pass (WIP only; clean `develop` is revive-green). `audit-security` shows zero blockers for the first snapshot, with 8 majors around credential lifecycle (ProxmoxConfig.Password still `string`, syscall.Exec env leak), TLS (VIP-probe InsecureSkipVerify, unpinned bootstrap oc download), and HTTP ignition. Sub-cluster storms from prior runs — observability redaction, subprocess stderr-dropping, modernization stdlib moves — all materially collapsed. **1 blocker**, **47 majors**, **41 minors**, **56 suggestions**. Ship-blocker candidate: 1 (`tst:daf5bee9`).

## Per-skill coverage

| Skill | Files touched | Findings | blocker | major | minor | suggestion | New | Resolved |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| `audit-api-design` | 19 | 19 | 0 | 0 | 5 | 14 | 2 | 4 |
| `audit-cli-ux` | 9 | 9 | 0 | 1 | 3 | 5 | 7 | 9 |
| `audit-code-smells` | 7 | 8 | 0 | 0 | 0 | 8 | 2 | 8 |
| `audit-concurrency` | 4 | 5 | 0 | 0 | 0 | 5 | 1 | 3 |
| `audit-dependencies` | 3 | 7 | 0 | 0 | 2 | 5 | 0 | 1 |
| `audit-documentation` | 21 | 27 | 0 | 24 | 3 | 0 | 27 | 2 |
| `audit-errors` | 8 | 8 | 0 | 0 | 1 | 7 | 2 | 8 |
| `audit-iac-and-shell` | 3 | 4 | 0 | 0 | 1 | 3 | 1 | 7 |
| `audit-modernization` | 4 | 6 | 0 | 0 | 1 | 5 | 4 | 10 |
| `audit-observability` | 10 | 10 | 0 | 0 | 8 | 2 | 2 | 5 |
| `audit-security` | 17 | 17 | 0 | 8 | 8 | 1 | 2 | 8 |
| `audit-state-and-recovery` | 5 | 6 | 0 | 2 | 3 | 1 | 2 | 3 |
| `audit-subprocess` | 3 | 3 | 0 | 2 | 1 | 0 | 1 | 9 |
| `audit-tests` | 14 | 16 | 1 | 13 | 2 | 0 | 7 | 39 |
| **TOTAL** | — | **145** | **1** | **47** | **41** | **56** | **60** | **116** |

## Top 20 findings (cross-audit, ranked)

Ranking formula: `severity × confidence × |LOC delta| × risk_weight` (low-risk ranks higher). Scaffolding-tagged items are excluded from the ranked table.

| Rank | ID | Skill | File:line | Severity | Conf | Risk | LOC | Fix |
|---:|---|---|---|---|---|---|---:|---|
| 1 | `tst:41a9d4eb:no-test-redact-handler` | tests | `internal/logutil/redact.go:30` | major | high | low | +150 | refactor |
| 2 | `tst:daf5bee9:no-test-kubeconfig-merge-full` | tests | `internal/cli/kubeconfig.go:77` | blocker | high | low | +90 | refactor |
| 3 | `tst:632c9087:no-test-buildlb-ingresscontroller` | tests | `internal/distribution/okd/postinstall/update_ingress.go:371` | major | high | low | +120 | refactor |
| 4 | `tst:15ba17da:no-test-destroy-orchestration` | tests | `internal/distribution/okd/destroy/steps.go:24` | major | high | medium | +150 | refactor |
| 5 | `tst:761e5126:no-test-removehaproxy` | tests | `internal/distribution/okd/postinstall/haproxy.go:23` | major | high | low | +90 | refactor |
| 6 | `tst:98723e5d:no-test-setup-cluster-access` | tests | `internal/distribution/okd/install/flux.go:50` | major | high | low | +80 | refactor |
| 7 | `tst:830d4653:no-test-packages-cleanup-guard` | tests | `internal/distribution/okd/cleanup/packages.go:59` | major | high | low | +75 | refactor |
| 8 | `tst:ae5b624c:test-missing-synctest-monitor` | tests | `internal/distribution/okd/install/monitor.go:43` | major | medium | medium | +120 | refactor |
| 9 | `tst:6b533f2d:no-test-approve-pending-csrs` | tests | `internal/cluster/k8s_csrs.go:51` | major | high | low | +50 | refactor |
| 10 | `tst:ab9b764a:no-test-installconfig-perms` | tests | `internal/distribution/okd/setup/ignition.go:34` | major | high | low | +50 | refactor |
| 11 | `tst:33579dd5:no-test-dnsmasq-cleanup-glob` | tests | `internal/distribution/okd/cleanup/services.go:137` | major | medium | low | +55 | refactor |
| 12 | `tst:33579dd5:no-test-cleanup-haproxy` | tests | `internal/distribution/okd/cleanup/services.go:50` | major | medium | low | +55 | refactor |
| 13 | `tst:98723e5d:no-test-add-kubeconfig-bashrc` | tests | `internal/distribution/okd/install/flux.go:93` | minor | high | low | +55 | refactor |
| 14 | `tst:25fa1be8:no-test-validateport-attacker` | tests | `internal/distribution/okd/firewall/firewall.go:124` | major | high | low | +35 | refactor |
| 15 | `state:93957c53:cleanup-no-confirm-cluster` | state-and-recovery | `internal/cli/cleanup.go:37` | major | high | low | +30 | refactor |
| 16 | `sub:7b2829bb:unbounded-output-buffer` | subprocess | `internal/executor/executor.go:224` | major | high | medium | +40 | refactor |
| 17 | `state:4c092fce:no-concurrent-run-guard` | state-and-recovery | `internal/infrastructure/terraform/terraform.go:109` | major | medium | medium | +50 | refactor |
| 18 | `ux:024a2c32:json-schema-doc-drift` | cli-ux | `docs/cli/json-schema.md:12` | major | high | low | +20 | refactor |
| 19 | `tst:29293401:no-test-haproxy-rollback` | tests | `internal/distribution/okd/setup/haproxy.go:87` | major | medium | high | +80 | refactor |
| 20 | `con:ae5b624c:synctest-opportunity` | concurrency | `internal/distribution/okd/install/monitor.go:52` | suggestion | medium | low | +60 | refactor |

## Ship-blockers (blocker + high-confidence + low/medium-risk)

### `tst:daf5bee9:no-test-kubeconfig-merge-full` — internal/cli/kubeconfig.go:77-125
**Skill:** `audit-tests`  **Severity:** blocker (rubric §4/data-loss — merging into an existing kubeconfig is a credential-mutating write; a bug in the current-context preservation silently flips kubectl's default cluster, and a perm regression to 0o644 world-reads every bearer token in the file. The sub-helper is tested but the wrapper's invariants (perms, current-context, three-list merge composition) aren't.)  **Risk:** low

**Smell:** mergeNamedList now has unit coverage (TestMergeNamedList) but mergeKubeconfig itself — the full merge pipeline including (a) source/dest YAML parse, (b) three-key merge (clusters/users/contexts), (c) current-context preservation invariant (set from src only when dest has none), (d) AtomicWrite at mode 0o600 — remains untested end-to-end. The current-context and 0o600 perm guarantees are the load-bearing invariants for kubectl-default-cluster preservation and on-disk kubeconfig perms.

**Fix:** Extend internal/cli/kubeconfig_test.go: TestMergeKubeconfig_PreservesCurrentContext — seed dest YAML with current-context=prod + one cluster 'prod', pass srcData with current-context=okd-test + clusters [okd-test,dev] via t.Setenv(KUBECONFIG, tmp) to redirect mergeTargetPath, call mergeKubeconfig(srcData), read-back YAML, assert current-context == 'prod' AND clusters contains both 'prod' and 'okd-test'. TestMergeKubeconfig_EmptyDestTakesSrcCurrentContext — empty dest → dest's current-context becomes src's. TestMergeKubeconfig_Perms — stat dest after merge, assert Mode().Perm() == 0o600.

**Must preserve:** The `existing == ""` guard on current-context; AtomicWrite at 0o600 for dest; the three-key merge composition.

## Findings by skill

### `audit-api-design` (19)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `opt-kubeconfig-env-binding` | minor | `internal/cluster/k8s.go:52` | option-consistency | cluster.NewK8sClient reads KUBECONFIG from os.Getenv at construction time, then builds the cmd runner. This couples the constructor to proce… |
| `zero-value-usable-cleanup` | minor | `internal/distribution/okd/cleanup/cleanup.go:50` | zero-value-usability | cleanup.Execute takes *Options whose zero Kind yields a bare '*errtypes.ConfigError{Msg: "unknown cleanup type: ..."}' with no sentinel call… |
| `opt-inconsistent-cfg-opts` | minor | `internal/distribution/okd/destroy/phase.go:40` | option-consistency | Phase NewOptions factory shapes still diverge across siblings. setup.DefaultOptions(projectRoot) takes ONLY projectRoot; install.NewOptions,… |
| `withenv-order-coupling` | minor | `internal/distribution/okd/okd.go:61` | option-consistency | okd.WithEnv still encodes an order-dependency contract in New: WithEnv may construct the executor before WithLogger runs, and New compensate… |
| `opt-inconsistent-terraform-ctors` | minor | `internal/infrastructure/terraform/terraform.go:109` | option-consistency | terraform package still exports two constructors — New(workDir, opts...) and NewWithVarFile(workDir, varFile, opts...) — that differ only in… |
| `export-no-caller-installed-lists` | suggestion | `internal/distribution/okd/cleanup/packages.go:34` | exported-surface | cleanup.InstalledPackages and cleanup.InstalledBinaries are exported but their only callers are the package-private Packages() function at l… |
| `export-no-caller-generate-summary` | suggestion | `internal/distribution/okd/cleanup/summary.go:11` | exported-surface | cleanup.GenerateSummary and cleanup.Summary struct are exported but the only caller is the package-private printSummary(). No external calle… |
| `export-no-caller-dns-config-helpers` | suggestion | `internal/distribution/okd/dns/dns.go:23` | exported-surface | dns.BuildConfigData, dns.ConfigName, and dns.WriteDnsmasqConfig remain exported with callers only inside the dns package. dns.GenerateBootst… |
| `ctx-not-first-write-dnsmasq` | suggestion | `internal/distribution/okd/dns/dnsmasq.go:54` | ctx-first | WriteDnsmasqConfig now takes ctx and checks ctx.Err() at entry (progress from prior run), but still does not thread ctx into os.MkdirAll / s… |
| `export-no-caller-configure` | suggestion | `internal/distribution/okd/firewall/firewall.go:97` | exported-surface | firewall.Configure and firewall.RemoveRules are exported but every in-tree caller uses ConfigureOKD / RemoveOKDRules. RemoveRules(HAProxyFro… |
| `export-no-caller-validateclusteraccess` | suggestion | `internal/distribution/okd/install/flux.go:15` | exported-surface | Phase.ValidateClusterAccess, Phase.SetupClusterAccess, Phase.SetupKubeconfig remain exported with callers only inside the same package's ste… |
| `concrete-return-k8s` | suggestion | `internal/distribution/okd/install/monitor.go:54` | interface-location | K8sClient is used in monitor.go only for ApprovePendingCSRs. Rather than accepting a concrete *cluster.K8sClient in MonitorInstallation, the… |
| `export-no-caller-external-tool-binaries` | suggestion | `internal/distribution/okd/phase/paths.go:96` | exported-surface | phase.ExternalToolBinaries has one in-tree caller (cleanup/packages.go:52). Exported for the sole purpose of avoiding a setup→cleanup import… |
| `stutter-postinstall-context` | suggestion | `internal/distribution/okd/postinstall/context.go:1` | exported-surface | postinstall.PostInstallContext stutters (package.PostInstall…). The struct is already suppressed with //nolint:revive and a 'rename deferred… |
| `export-no-caller-removehaproxy` | suggestion | `internal/distribution/okd/postinstall/haproxy.go:23` | exported-surface | postinstall.Phase.RemoveHAProxy is exported but the only caller is the package-private finalizeIngress path in update_ingress.go:214. No ext… |
| `export-no-caller-getlatestforminor` | suggestion | `internal/distribution/okd/releases/okd.go:45` | exported-surface | OKDVersionFetcher.GetLatestStable and OKDVersionFetcher.GetLatestForMinor remain exported with no caller. Only FetchVersions is invoked (int… |
| `mix-default-new-naming` | suggestion | `internal/distribution/okd/setup/phase.go:34` | option-consistency | setup.DefaultOptions continues the Default* naming pattern common for 'zero-arg constructor of a defaulted options struct'. Other phase pack… |
| `iface-fragmented-step` | suggestion | `internal/distribution/step.go:31` | interface-location | Step / Skipper / FatalChecker / StepCallbacks remain four interfaces that ProvisioningStep always composes together. The builtStep impl impl… |
| `iface-in-consumer` | suggestion | `internal/infrastructure/proxmox/proxmox.go:33` | interface-location | Provider struct still has public methods (Connect, Disconnect, Provision, PlanOnly) but no consumer-side interface. cli/helpers.go and insta… |

### `audit-cli-ux` (9)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `json-schema-doc-drift` | major | `docs/cli/json-schema.md:12` | json-stability | docs/cli/json-schema.md documents field shapes that do not match what the code emits. `okdctl status --format=json` is documented with clust… |
| `readme-flag-drift` | minor | `docs/cli/okdctl_destroy.md:26` | help-text | Generated CLI reference for `okdctl destroy` is stale: commit afsd79b added --skip-terraform, --skip-cleanup, --skip-firewall to destroy.go,… |
| `dry-run-yes-short-circuit` | minor | `internal/cli/deploy.go:78` | flag-conventions | runDeploy checks deployYes before deployDryRun and returns after saving the config, so `okdctl deploy --yes --dry-run` silently skips the dr… |
| `json-key-hyphenated` | minor | `internal/cli/status.go:338` | json-stability | runDescribeAddon emits JSON with a hyphen-cased key `display-name` while every other field in the same payload and every other JSON endpoint… |
| `sig-not-handled-preflight` | suggestion | `cmd/okdctl/main.go:20` | signals | main() calls preflight() before cli.Execute(); signal.Notify setup lives inside internal/cli/root.go:execute(). If the user hits Ctrl-C duri… |
| `cleanup-no-dry-run` | suggestion | `internal/cli/cleanup.go:18` | flag-conventions | cleanupCmd has no --dry-run flag while its destructive siblings (deploy, destroy, update-ingress) all do. cleanup removes packages, dnsmasq/… |
| `completion-use-bracket-optional` | suggestion | `internal/cli/completion.go:11` | help-text | completionCmd.Use is 'completion [bash\|zsh\|fish\|powershell]' — square brackets per man(1) convention mean optional, but cobra.ExactArgs(1) r… |
| `releases-show-no-completion` | suggestion | `internal/cli/releases.go:52` | help-text | addon install/uninstall and describe-addon gained ValidArgsFunction for tab-completion; releasesShowCmd still has none. Tab-completing `okdc… |
| `exit-code-bsd-sysexits-partial` | suggestion | `internal/cli/root.go:144` | exit-codes | exitCodeFor maps ConfigError=2 (not EX_DATAERR=65 or EX_CONFIG=78), NetworkError=3 (not EX_UNAVAILABLE=69), ClusterError=4 (not EX_UNAVAILAB… |

### `audit-code-smells` (8)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `scaffolding-registry-api` | suggestion | `internal/addon/registry.go:86` | helper-package-no-value | addon.IsRegistered has no callers. However it is a clean symmetric API pair with Register/Get/All/Names/Enabled (if Register exists, IsRegis… |
| `yaml-tree-walk-repeat-assertion` | suggestion | `internal/cli/kubeconfig.go:141` | interfaceany-lazy | mergeNamedList has four nested type-assertion chains to walk a generic YAML tree (any → []any → map[string]any → map[string]any['name'] → st… |
| `helper-pkg-thin-wrap` | suggestion | `internal/distribution/okd/packages/packages.go:1` | helper-package-no-value | Package `packages` wraps `platform.PackageManager.Install`/`Remove` with an extra logger.Info() envelope and a single `fmt.Errorf` rewrap. T… |
| `enum-via-sscanf-int-parse` | suggestion | `internal/distribution/okd/releases/types.go:59` | stringified-numbers | OKDVersion.Major() and OKDVersion.Minor() parse the Version string via fmt.Sscanf on every call. ShortVersion calls both (two parses per cal… |
| `build-role-helper-near-duplicate` | suggestion | `internal/distribution/okd/setup/terraform.go:20` | premature-abstraction | buildISOStrings and buildNodeNames in setup/terraform.go are structurally identical: allocate []string of length count, loop `for i := range… |
| `named-return-unnecessary` | suggestion | `internal/distribution/okd/setup/terraform.go:36` | premature-abstraction | getDiskSizes returns `(cpDisk, workerDisk, workerDataDisk, masterDataDisk int)` — four unnamed integers with no semantic ordering. The named… |
| `stepbuilder-build-no-callers` | suggestion | `internal/distribution/step.go:155` | helper-package-no-value | distribution.StepBuilder.Build() has no external callers; every production path goes through BuildSteps → MustBuild, and MustBuild is Build'… |
| `query-match-mini-dsl` | suggestion | `internal/platform/packages.go:100` | premature-abstraction | Manager.IsInstalled uses a bespoke `queryMatch` string substring to distinguish "installed" from "purged" on dpkg output. The logic is corre… |

### `audit-concurrency` (5)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `go-no-wait` | suggestion | `internal/cli/confirm.go:22` | goroutine-lifetime | promptForConfirmation spawns a reader goroutine that blocks on bufio.Reader.ReadString, races against ctx.Done, and on ctx cancel the gorout… |
| `lock-held-during-write` | suggestion | `internal/deploymetrics/metrics.go:75` | waitgroup-vs-errgroup | Handler holds r.mu.Lock() across fmt.Fprint(w, b.String()) — writing to an http.ResponseWriter under the mutex. A slow Prometheus scraper or… |
| `synctest-opportunity` | suggestion | `internal/distribution/okd/install/monitor.go:52` | time-sleep-retry | MonitorInstallation has a ticker-driven CSR-approval loop, a reap timer, and ctx.Done/DeadlineExceeded paths — exactly the shape testing/syn… |
| `go-leak-on-error` | suggestion | `internal/distribution/okd/install/monitor.go:65` | goroutine-lifetime | MonitorInstallation spawns a goroutine holding installCmd.Wait(). On ctx cancel the function calls killInstall, waits up to 30s via reapTime… |
| `go-no-wait` | suggestion | `internal/version/updatecheck.go:40` | goroutine-lifetime | BackgroundCheck spawns a fire-and-forget goroutine that runs runCheck(ctx); printUpdateNotice in cli/root.go waits at most 100ms before retu… |

### `audit-dependencies` (7)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `yaml-quad-engines` | minor | `go.mod:20` | duplicate-engine | Four YAML engines in the tree: sigs.k8s.io/yaml (direct), go.yaml.in/yaml/v2 (via k8s), go.yaml.in/yaml/v3 (via cobra/doc + kube-openapi), g… |
| `ultraviolet-pseudo-version` | minor | `go.mod:27` | pin-stability | github.com/charmbracelet/ultraviolet is pinned to a pseudo-version (commit SHA, not a tagged release) — the project has never cut a tag. Pul… |
| `workflow-pin-hygiene-clean` | suggestion | `.github/workflows/ci.yml:1` | pin-stability | Pin hygiene audit: every GitHub Action in .github/workflows/ is pinned by full 40-char SHA with the version tag in a trailing comment (actio… |
| `goreleaser-action-version-tag` | suggestion | `.github/workflows/release.yml:25` | pin-stability | goreleaser-action is SHA-pinned (good), but the version parameter it resolves IS a tag, not a SHA — version: v2.15.2 in both release.yml and… |
| `copyleft-audit-clean` | suggestion | `go.mod:1` | license-compat | License compatibility audit: NO copyleft (GPL/AGPL/LGPL) or custom/unclear licenses in the transitive dep tree. All direct and indirect deps… |
| `go-yaml-in-fork-risk` | suggestion | `go.mod:58` | maintenance-signal | go.yaml.in/yaml/v2 and go.yaml.in/yaml/v3 are a vanity-domain fork of the original gopkg.in/yaml.v{2,3}. The domain (go.yaml.in) is a 2024+ … |
| `golang-x-exp-stale` | suggestion | `go.mod:60` | justified-version-floor | golang.org/x/exp pinned at v0.0.0-20231006140011 (Oct 2023) — almost 2.5 years old. Pulled transitively by charm.land/log/v2, which only imp… |

### `audit-documentation` (3)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `addons-buildopaquesecret-sig` | minor | `docs/architecture/addons.md:141` | readme-drift | docs/architecture/addons.md documents `addon.BuildOpaqueSecret(name, namespace, data)` but the actual helper signature in internal/addon/hel… |
| `wizard-registration-stale` | minor | `docs/architecture/wizard.md:38` | readme-drift | docs/architecture/wizard.md tells step authors to 'register the step in the wizard assembly in internal/tui/wizard/wizard.go' — but `wizard.… |
| `destroy-cli-ref-stale` | minor | `docs/cli/okdctl_destroy.md:26` | readme-drift | Generated CLI reference for `okdctl destroy` is stale: missing three flags added in commit afa579b (`--skip-terraform`, `--skip-cleanup`, `-… |

### `audit-errors` (8)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `typed-err-fallthrough` | minor | `internal/infrastructure/proxmox/proxmox.go:181` | domain-vocabulary | Provider.Provision and Provider.retrieveProvisionResult still raise bare fmt.Errorf for config-class / cluster-runtime failures ('no VMs pro… |
| `wrap-tool-prereq-untyped` | suggestion | `internal/addon/catalog/flux/flux.go:72` | domain-vocabulary | Flux.Install returns a bare fmt.Errorf('helm is required to install Flux') when helm is missing. The message is user-friendly but the error … |
| `errors-join-ctx-lost` | suggestion | `internal/addon/manager.go:83` | cancellation-identity | InstallAll aggregates failures via errors.Join(errs...) after wrapping each with ClusterError at installAndVerify:120. Good pattern. BUT the… |
| `ctx-err-check-on-ctx` | suggestion | `internal/cli/root.go:110` | cancellation-identity | execute() still checks `if ctx.Err() != nil` to decide whether to return 130 (SIGINT) or 143 (SIGTERM). This works today because the hand-ro… |
| `vocab-gap-cert-pending` | suggestion | `internal/errtypes/errtypes.go:1` | domain-vocabulary | errtypes vocabulary covers Config/Network/Cluster/Auth but still has NO typed error for two concepts the phases hit repeatedly: (a) 'recover… |
| `typed-no-error-iface` | suggestion | `internal/executor/executor.go:184` | sentinel-vs-typed | executor.ExitError doc still claims 'errors.Is to compare against Unwrap chain values' but the type has no Unwrap() method and no Err field.… |
| `vocab-ad-hoc-sentinel` | suggestion | `internal/infrastructure/proxmox/types.go:9` | domain-vocabulary | ErrNotConnected and ErrTerraformNotConfigured are package-level sentinels defined outside errtypes. Still never matched with errors.Is anywh… |
| `err-stringified-loses-type` | suggestion | `internal/netutil/ip.go:43` | wrapping | Four sites still use `if err != nil \|\| !X.Is4()` and return a synthetic fmt.Errorf that drops the netip.ParseAddr / netip.ParsePrefix error … |

### `audit-iac-and-shell` (4)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `ci-no-tflint-tfsec` | minor | `.github/workflows/ci.yml:97` | hcl-credential-hygiene | `validate-terraform` + `lint-terraform` jobs now run `terraform fmt`, `terraform validate`, and `tflint -f compact` — but no secret/policy s… |
| `tflint-no-config` | suggestion | `.github/workflows/ci.yml:102` | hcl-provider-hygiene | CI runs `tflint --init && tflint -f compact` with no `.tflint.hcl` config file in either module or environment directory. Without a config, … |
| `hcl-no-prevent-destroy-masters` | suggestion | `infrastructure/terraform/modules/proxmox-okd/main.tf:140` | hcl-destroy-ordering | Master VMs (OKD control plane carrying etcd quorum state) have no `lifecycle { prevent_destroy = true }` guard. A misconfigured `terraform a… |
| `sh-posix-not-bash` | suggestion | `scripts/install.sh:1` | install-sh-fail-closed | Shebang `#!/bin/sh` constrains the script to POSIX sh (dash on Debian/Ubuntu, ash on Alpine), which prevents unconditional `set -o pipefail`… |

### `audit-modernization` (6)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `use-map-index` | minor | `internal/cli/status.go:97` | slices-maps | `statusNode.role()` iterates every key of `Labels` to check for two specific well-known strings. This is a map lookup dressed as a scan — O(… |
| `use-strings-lines` | suggestion | `internal/cli/status.go:171` | range-idioms | `for _, line := range strings.Split(strings.TrimSpace(coRaw), "\n")` materializes the split slice only to walk it. Go 1.24's `strings.Lines`… |
| `use-slices-max` | suggestion | `internal/distribution/okd/setup/coreos.go:70` | slices-maps | One of two near-identical blocks in `findOrDownloadFCOSISO` still does `slices.Sort(matches); matches[len(matches)-1]` to fetch the lexicogr… |
| `use-slices-concat` | suggestion | `internal/platform/packages.go:101` | slices-maps | `append(append([]string{}, m.queryArgs...), pkg)` nests two `append`s to clone-then-extend a slice. Go 1.22's `slices.Concat` expresses the … |
| `use-builtin-max-innerwidth` | suggestion | `internal/tui/layouts.go:54` | any-interface-builtins | Two sequential `if X > innerWidth { innerWidth = X }` blocks compute a running max over two candidates. Go 1.21's `max` builtin collapses bo… |
| `use-builtin-max-padding` | suggestion | `internal/tui/layouts.go:100` | any-interface-builtins | `padding := innerWidth - lineWidth; if padding < 0 { padding = 0 }` is a hand-rolled `max(padding, 0)` — the exact floor `max` was added (Go… |

### `audit-observability` (10)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `level-warn-help-text` | minor | `internal/addon/catalog/secretstore/secretstore.go:122` | level-discipline | secretstore.installPrereqCheck still logs multi-line HOW-TO guides (onepassword: 6 Warn lines, vault: 3 Warn lines, bitwarden: 3 Warn lines)… |
| `handler-no-tty-switch` | minor | `internal/cli/logging.go:35` | handler-setup | configureLogging still does not auto-select JSON format when stderr is not a TTY. Operators piping `okdctl deploy 2>&1 \| jq .` get charmlog … |
| `err-stringified-into-label` | minor | `internal/distribution/okd/destroy/steps.go:32` | field-stability | destroy.steps.go builds its per-step OnError callback as `phase.WarnOnError(p.Log, label+": "+err.Error())(err)`, which concatenates err.Err… |
| `inconsistent-domain-prefix-keys` | minor | `internal/distribution/okd/postinstall/update_ingress.go:140` | field-stability | The codebase still leans on the `prefix: message` convention ('update-ingress:', 'haproxy:', 'kubevip:', 'cluster:', 'cleanup:', 'terraform:… |
| `duplicate-iso-exists-log` | minor | `internal/distribution/okd/setup/coreos.go:59` | log-once | coreos.go logs `coreos: found existing iso at X` (L59, L73) and `coreos: iso already exists at X` (L201, L265) in four distinct sites. A sin… |
| `span-no-start-end-per-step` | minor | `internal/distribution/orchestrator.go:113` | span-retry-boundary | Orchestrator.executeStep still does not emit a structured start/finish log pair per step. Skipping is logged (L90) but success/duration is n… |
| `executor-no-output-span` | minor | `internal/executor/executor.go:213` | span-retry-boundary | executor.run and RunInteractive still only log `+ <name> <args>` at Debug when Verbose is true — nothing bookends the call in the structured… |
| `message-embedded-counts` | minor | `internal/infrastructure/proxmox/proxmox.go:217` | field-stability | The prior audit flagged three terraform count-in-message lines in proxmox.go; L158 and L185 are now structured (`"count", n`) but L217 remai… |
| `root-error-stringified` | suggestion | `internal/cli/root.go:187` | field-stability | The ctx-done-miss branch at L120 was migrated to structured form `tui.Error("command failed", tui.LF("err", err))` — prior audit's core case… |
| `monitor-retry-log-per-tick` | suggestion | `internal/distribution/okd/install/monitor.go:119` | log-once | MonitorInstallation's CSR approval tick runs every 30s for up to 60 minutes. On each tick: on error it Warns structured, on approved>0 it In… |

### `audit-security` (17)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `ssh-keyscan-tofu` | major | `internal/addon/catalog/flux/flux.go:329` | input-validation | createDeployKeySecret derives a git host via gitHost(repoURL), then runs `ssh-keyscan host` and stuffs the output verbatim into the flux sec… |
| `cred-as-string-in-config` | major | `internal/config/cluster.go:107` | credentials | ProxmoxConfig.Password and ProxmoxConfig.APIToken are typed as `string` (with `json:"-"`). The credentials.GetProxmoxCredentials legacy fall… |
| `cred-string-copy-envfile` | major | `internal/credentials/envfile.go:42` | credentials | WriteEnvFile converts password and API-token []byte to an immutable Go string via string concatenation before calling AtomicWrite. The strin… |
| `cred-string-copy-env` | major | `internal/credentials/proxmox.go:113` | credentials | ProxmoxCredentials.Env() builds subprocess env entries via string concatenation: "PROXMOX_VE_PASSWORD="+string(c.Password). The resulting Go… |
| `ignition-dir-perms` | major | `internal/distribution/okd/setup/apache.go:28` | file-toctou | ensureIgnitionDir creates /var/www/html/ignition at 0o755 and then explicitly re-chmods to 0o755 if pre-existing. The ignition files inside … |
| `http-ignition-pullsecret` | major | `internal/distribution/okd/setup/phase.go:44` | tls-network | BuildIgnitionURL returns http:// (not https://) for the bastion-hosted ignition endpoint. The generated bootstrap/master/worker .ign files e… |
| `bootstrap-oc-no-integrity` | major | `internal/distribution/okd/setup/release_extract.go:24` | tls-network | bootstrapOC downloads oc.tar.gz from mirror.openshift.com with no checksum or cosign signature verification. The docstring admits 'no upstre… |
| `tls-insecure-vip-probe` | major | `internal/httputil/httputil.go:22` | tls-network | NewInsecure returns an http.Client with InsecureSkipVerify=true. It is used at two cluster-credentialed call sites (postinstall/haproxy.go:6… |
| `secretstore-plaintext-disk` | minor | `internal/addon/catalog/secretstore/secretstore.go:253` | credentials | The secretstore addon reads 1password-credentials.json and 1password-token.txt (plus the vault/bitwarden equivalents) from automation/config… |
| `debug-bundle-redact-partial` | minor | `internal/cli/config.go:65` | redaction | redactConfig in cli/config.go only masks Provider.Proxmox.TokenID and leaves every other config field unchanged. Password and APIToken carry… |
| `syscall-exec-env-leak` | minor | `internal/cli/elevation.go:54` | privilege-escalation | ensureRoot re-execs via syscall.Exec(sudoPath, args, os.Environ()). The full inherited environment is handed to sudo → the new okdctl proces… |
| `bashrc-no-nofollow` | minor | `internal/distribution/okd/install/flux.go:93` | file-toctou | addKubeconfigToBashrc opens ~/.bashrc with os.OpenFile(O_APPEND\|O_WRONLY, 0o644) under the sudo re-exec (running as root, HOME resolved via … |
| `ssh-accept-new-proxmox` | minor | `internal/distribution/okd/phase/ssh.go:27` | input-validation | SSHRun uses `-o StrictHostKeyChecking=accept-new` on every SSH to the Proxmox host. First-contact TOFU. okdctl destroy reaches for the Proxm… |
| `scp-accept-new-proxmox` | minor | `internal/distribution/okd/setup/upload.go:42` | input-validation | uploadISOsViaSCP uses the same `StrictHostKeyChecking=accept-new` TOFU pattern for scp. A MITM on the first bastion→Proxmox scp can coerce t… |
| `env-append-os-environ` | minor | `internal/executor/executor.go:85` | credentials | Executor now applies a defaultEnvAllowlist (good — previously-flagged broadcast of unrelated env vars is closed). But PROXMOX_ is in the pre… |
| `chowntree-symlink-audit` | minor | `internal/system/elevation.go:100` | privilege-escalation | ChownTreeToInvokingUser uses filepath.WalkDir + os.Lchown (symlink-safe). The docstring explicitly requires the caller to only pass paths wh… |
| `kubeconfig-env-leak` | suggestion | `internal/distribution/okd/install/phase.go:151` | credentials | SetupKubeconfig appends `KUBECONFIG=<path>` to p.Exec.Env, making the kubeconfig path visible to every subprocess the executor spawns from t… |

### `audit-state-and-recovery` (6)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `cleanup-no-confirm-cluster` | major | `internal/cli/cleanup.go:37` | destroy-safety | `okdctl cleanup` has only `--yes` with no typo-guard against the wrong config. Unlike `okdctl destroy` which requires `--confirm-cluster=<na… |
| `no-concurrent-run-guard` | major | `internal/infrastructure/terraform/terraform.go:109` | tf-state-atomicity | Two concurrent `okdctl deploy` or `okdctl destroy` invocations in the same project root both point at `infrastructure/terraform/environments… |
| `postinstall-no-rollback-path` | minor | `internal/distribution/okd/postinstall/steps.go:42` | crash-recoverability | postinstall steps (cleanup-bootstrap, deploy-production-dns) are NonFatal and mutate cluster-external state (bootstrap VM destroyed via targ… |
| `no-resume-checkpoint` | minor | `internal/distribution/step.go:178` | phase-idempotency | StepDef has no 'already-done' precondition hook or checkpoint. If okdctl crashes mid-setup, the next run starts from step 1, repeating work … |
| `provision-leaves-tfplan` | minor | `internal/infrastructure/proxmox/proxmox.go:149` | crash-recoverability | `Provider.Provision` writes `<workDir>/tfplan` via Plan then applies it, but never sweeps the plan file on success or failure — only `destro… |
| `proxmox-no-retry-layer` | suggestion | `internal/infrastructure/proxmox/proxmox.go:131` | proxmox-api-idempotency | Provider.Provision delegates 100% to terraform — no Go-side retry on transient Proxmox API failures, no 409-already-exists handling beyond w… |

### `audit-subprocess` (3)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `unbounded-output-buffer` | major | `internal/executor/executor.go:224` | io-handling | The canonical Executor.Run still buffers stdout and stderr into unbounded bytes.Buffer with no streaming option. Long-running commands route… |
| `terraform-buffered-through-executor` | major | `internal/infrastructure/terraform/terraform.go:138` | io-handling | terraform Executor.run still routes init/apply/destroy through the buffered internal/executor Run — apply of a 6-VM Proxmox cluster routinel… |
| `systemd-stderr-dropped` | minor | `internal/system/systemd.go:36` | io-handling | ManageService runs systemctl enable/disable/start/stop/restart/reload via exec.CommandContext(...).Run() with both stdout and stderr left ni… |

### `audit-tests` (16)

| ID | Sev | File:line | Cluster | Smell |
|---|---|---|---|---|
| `no-test-kubeconfig-merge-full` | blocker | `internal/cli/kubeconfig.go:77` | cred-path-untested | mergeNamedList now has unit coverage (TestMergeNamedList) but mergeKubeconfig itself — the full merge pipeline including (a) source/dest YAM… |
| `no-test-approve-pending-csrs` | major | `internal/cluster/k8s_csrs.go:51` | canonical-helper-untested | ApprovePendingCSRs drives MonitorInstallation's CSR-approval loop. No test covers (a) PendingCSRs returns [] → (0, nil) fast path, (b) non-e… |
| `no-test-packages-cleanup-guard` | major | `internal/distribution/okd/cleanup/packages.go:59` | destructive-untested | cleanup.Packages composes ResolveBinDir → filepath.Join → refuseCriticalPath → os.RemoveAll for each installer-managed binary (yq/helm/sops/… |
| `no-test-cleanup-haproxy` | major | `internal/distribution/okd/cleanup/services.go:50` | destructive-untested | cleanup.HAProxy deletes the live haproxy config, globs *.backup.* siblings, removes okdctl firewall rules, releases the bastion VIP, and uni… |
| `no-test-dnsmasq-cleanup-glob` | major | `internal/distribution/okd/cleanup/services.go:137` | destructive-untested | Dnsmasq runs os.RemoveAll against paths produced by filepath.Glob("/etc/dnsmasq.d/okd-*.conf") and a secondary backup glob. Each match is gu… |
| `no-test-destroy-orchestration` | major | `internal/distribution/okd/destroy/steps.go:24` | destructive-untested | destroySteps orchestrates Terraform destroy + ISO removal + file cleanup + firewall cleanup, now with the 'failures' tracker and the SkipTer… |
| `no-test-validateport-attacker` | major | `internal/distribution/okd/firewall/firewall.go:124` | trust-boundary-untested | validatePort is the explicit defense-in-depth guard preventing Port.Protocol from flowing unchecked into fmt.Sprintf("%d/%s", ...) and onwar… |
| `no-test-setup-cluster-access` | major | `internal/distribution/okd/install/flux.go:50` | cred-path-untested | SetupClusterAccess installs the generated kubeconfig into ~/.kube/config under the invoking user's home (after sudo re-exec), backing up any… |
| `test-missing-synctest-monitor` | major | `internal/distribution/okd/install/monitor.go:43` | canonical-helper-untested | MonitorInstallation is the canonical ctx-cancel-reap-goroutine pattern for openshift-install monitoring. Has a ticker-driven CSR approval lo… |
| `no-test-removehaproxy` | major | `internal/distribution/okd/postinstall/haproxy.go:23` | destructive-untested | RemoveHAProxy calls os.RemoveAll(phase.DefaultHAProxyConfigPath) (= /etc/haproxy/haproxy.cfg) then tears down firewall rules, the bastion VI… |
| `no-test-buildlb-ingresscontroller` | major | `internal/distribution/okd/postinstall/update_ingress.go:371` | destructive-untested | convertToLoadBalancer is a destructive conversion (`oc delete ingresscontroller` then `oc create` a rebuilt one) with an explicit rollback p… |
| `no-test-haproxy-rollback` | major | `internal/distribution/okd/setup/haproxy.go:87` | destructive-untested | ConfigureHAProxy writes to /etc/haproxy/haproxy.cfg — a root-required file on the live system — and has a rollback path that restores from b… |
| `no-test-installconfig-perms` | major | `internal/distribution/okd/setup/ignition.go:34` | cred-path-untested | GenerateInstallConfig reads the pull-secret and writes install-config.yaml (containing the raw pull-secret JSON) at mode 0o600 via AtomicWri… |
| `no-test-redact-handler` | major | `internal/logutil/redact.go:30` | canonical-helper-untested | RedactHandler is the canonical slog redaction middleware — CLAUDE.md §credentials-and-secrets explicitly calls it out as the mechanism "so c… |
| `no-test-add-kubeconfig-bashrc` | minor | `internal/distribution/okd/install/flux.go:93` | cred-path-untested | addKubeconfigToBashrc appends `export KUBECONFIG=<path>` to the invoking user's ~/.bashrc. It preserves the existing file mode explicitly (d… |
| `no-test-writeasinvoking` | minor | `internal/system/elevation.go:82` | cred-path-untested | WriteAsInvokingUser combines AtomicWrite + chown-back. The "parent dir chowned iff it did not pre-exist" logic (line 84-86 + 94-96) is a sub… |

## Seams resolved

Total rows with `seam != none`: **35**. These rows declare co-ownership with another audit skill and rely on `seams.md` for ownership assignment. Seam cross-references via `related` are not required where the ownership is pure (per-site vs policy, sink vs chain).

Distribution:
- `audit-api-design`: 3
- `audit-cli-ux`: 4
- `audit-concurrency`: 3
- `audit-documentation`: 3
- `audit-errors`: 3
- `audit-modernization`: 2
- `audit-observability`: 4
- `audit-security`: 3
- `audit-state-and-recovery`: 4
- `audit-subprocess`: 4
- `audit-tests`: 2

## Scaffolding items (not ranked)

Per MEMORY.md §scaffolding: symbols shaped like future-CLI verbs or symmetric package APIs stay even when `deadcode` flags them. **9** findings carry `scaffolding: true` (max severity suggestion).

| ID | File:line | Reason |
|---|---|---|
| `api:25fa1be8:export-no-caller-configure` | `internal/distribution/okd/firewall/firewall.go:97` | symmetric-api |
| `api:66f217c9:export-no-caller-getlatestforminor` | `internal/distribution/okd/releases/okd.go:45` | symmetric-api |
| `api:98723e5d:export-no-caller-validateclusteraccess` | `internal/distribution/okd/install/flux.go:15` | future-cli-verb |
| `api:830d4653:export-no-caller-installed-lists` | `internal/distribution/okd/cleanup/packages.go:34` | future-cli-verb |
| `api:761e5126:export-no-caller-removehaproxy` | `internal/distribution/okd/postinstall/haproxy.go:23` | future-cli-verb |
| `smell:2be6306e:scaffolding-registry-api` | `internal/addon/registry.go:86` | symmetric-api |
| `smell:4f69fc9d:stepbuilder-build-no-callers` | `internal/distribution/step.go:155` | symmetric-api |
| `err:d6b325cb:vocab-ad-hoc-sentinel` | `internal/infrastructure/proxmox/types.go:9` | symmetric-api |
| `err:a4001485:vocab-gap-cert-pending` | `internal/errtypes/errtypes.go:1` | symmetric-api |

## Linter-config-bug candidates

**17** findings have `adjacent_linter_enabled: true` — the linter is ALREADY enabled in `.golangci.yml` but missed these specific sites. These are candidates for a single `.golangci.yml` tuning PR. Aggregated artifact: `.claude/audits/linter-config-bugs.jsonl`.

| Linter | Count | Example |
|---|---:|---|
| `unused` | 8 | `api:25fa1be8:export-no-caller-configure` |
| `gocritic` | 4 | `mod:0934cf1b:use-slices-concat` |
| `revive` | 2 | `api:beabab0c:mix-default-new-naming` |
| `dupl` | 1 | `smell:c5e5c304:build-role-helper-near-duplicate` |
| `shellcheck:SC2039` | 1 | `iac:e076e43c:sh-posix-not-bash` |
| `tflint` | 1 | `iac:b803fcb7:tflint-no-config` |

## Artifact index

- `.claude/audits/audit-api-design.jsonl` — 19 findings
- `.claude/audits/audit-cli-ux.jsonl` — 9 findings
- `.claude/audits/audit-code-smells.jsonl` — 8 findings
- `.claude/audits/audit-concurrency.jsonl` — 5 findings
- `.claude/audits/audit-dependencies.jsonl` — 7 findings
- `.claude/audits/audit-documentation.jsonl` — 3 findings
- `.claude/audits/audit-errors.jsonl` — 8 findings
- `.claude/audits/audit-iac-and-shell.jsonl` — 4 findings
- `.claude/audits/audit-modernization.jsonl` — 6 findings
- `.claude/audits/audit-observability.jsonl` — 10 findings
- `.claude/audits/audit-security.jsonl` — 17 findings
- `.claude/audits/audit-state-and-recovery.jsonl` — 6 findings
- `.claude/audits/audit-subprocess.jsonl` — 3 findings
- `.claude/audits/audit-tests.jsonl` — 16 findings
- `.claude/audits/linter-config-bugs.jsonl` — 17 findings (derived)
- `.claude/audits/history/audit-*-2026-04-21.jsonl` — prior-run snapshots (14 files)

## Footer

- Total findings across all audits: **145** (blocker 1, major 47, minor 41, suggestion 56)
- Note: `audit-documentation` reported 3 findings in its summary but emitted 27. The extra 24 are `exported-doc-missing` findings where a recent WIP comment-hygiene pass stripped revive-required exported docs — these overlap with `revive:exported` (which is enabled) and would fire on commit. Counted separately from the "headline 3" but tracked in aggregate totals. See §10 overlap note below.
- Skills that failed to run: **0**
- Schema validation failures auto-repaired: **5** (`smell`/`severity_reason` length caps — trimmed in-place to preserve findings)
- Duplicate IDs dropped: **0**
- Seam deferrals: **35**
- Prior-run delta: new **60** / recurring **85** / resolved **116**
- CI baseline: clean `develop` is `0 issues`. Working tree has **50 revive warnings** in in-scope files (all from uncommitted WIP — findings here reflect on-disk state).
- MEMORY.md honored: go-proxmox v0.x, gorilla/websocket (transitive), --color flag, Linux-only compat — none re-flagged.
