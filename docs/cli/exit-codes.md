# okdctl exit codes

Every `okdctl` invocation exits with one of the codes below, making it safe
to use in scripts that branch on failure type.

| Code | BSD name     | Trigger                                                      |
|------|--------------|--------------------------------------------------------------|
| 0    | EX_OK        | success                                                      |
| 1    | —            | unclassified error (unknown subcommand, arg-count violation) |
| 2    | —            | configuration error (parse failure, schema mismatch, doctor preflight [fail]) |
| 3    | —            | network error (HTTP, DNS, TLS, download failure)             |
| 4    | —            | cluster error (oc/kubectl failure, install timeout)          |
| 5    | —            | auth error (proxmox token rejected, insecure file perms)     |
| 6    | —            | `doctor` preflight warn-only: one or more checks reported `[warn]` and none reported `[fail]` |
| 7    | —            | `plan` found drift: a create/update/replace/delete is pending against the current configuration |
| 64   | EX_USAGE     | unknown flag, or an invalid flag combination detected at runtime (e.g. `--target` without `--confirm-cluster`) |
| 65   | EX_DATAERR   | pull secret file exists but is not valid JSON                |
| 66   | EX_NOINPUT   | configuration file not found on disk                         |
| 70   | EX_SOFTWARE  | internal error: a panic was caught at top level (a bug in okdctl — the stack trace goes to stderr and the run log) |
| 71   | EX_OSERR     | sudo not found on PATH (requires root operation)             |
| 130  | —            | interrupted by SIGINT (Ctrl-C)                               |
| 143  | —            | terminated by SIGTERM                                        |

Codes 65, 66, and 71 are granular refinements within the broader categories
2 (config) and 5 (auth). A script that only checks for non-zero exit is
unaffected; a script that branches on code 2 or 5 should also handle 65/66/71.

Code 6 is `doctor`-specific and sits outside the ConfigError/NetworkError/
ClusterError/AuthError/UsageError hierarchy below: no other command emits
it, and it is not a refinement of any broader category the way 65/66/71 are.
It exists purely so a cron job can tell "clean" (0), "needs attention but
not blocking" (6), and "blocking" (2) apart without parsing output.

Code 7 is `plan`-specific and follows the same pattern as code 6: drift is
not a failure (`okdctl plan` ran successfully and reported an accurate
result), so it gets its own dedicated code rather than folding into
ConfigError(2). A script can tell "clean" (0) apart from "drifted, run
`okdctl deploy` to reconcile" (7) apart from "plan itself failed" (2/3/4/5)
without parsing output. `deploy --dry-run` shares the same plan-preview
machinery but keeps its pre-existing exit-0-on-drift contract — only `plan`
gained the code.

When an error wraps more than one typed category (e.g. a `ClusterError`
wrapping a `ConfigError` produced during a failed reload), resolution is not
"outermost wins": sentinels (65/66/71) outrank every category, and within
categories the precedence is `Config` (2) > `Network` (3) > `Cluster` (4) >
`Auth` (5) > `Usage` (64) — whichever type is present anywhere in the chain,
in that order, determines the exit code.

Commands that do not require root (`status`, `config`, `kubeconfig`, and
others) exit with code 5 when invoked under `sudo` or as root — the binary
refuses with "do not run as root/sudo; this tool escalates internally".
Root-requiring commands (`deploy`, `destroy`, `cleanup`, `update-ingress`)
must be invoked as a regular user; the binary self-elevates via an internal
`sudo` re-exec so the privileged body runs as euid=0.

## ConfigError vs UsageError

Code 2 (ConfigError) covers problems with the content on disk: a config file
that fails to parse, fails schema validation, or a `doctor` check that reports
`[fail]`. `doctor` exits 0 only when every check passes, 6 when one or more
checks report `[warn]` and none report `[fail]`, and 2 on any `[fail]`,
whether from the host-preflight checks or the day-2 `cluster` section (a
Degraded ClusterOperator, a NotReady node, unhealthy etcd, or an expired
kube-apiserver-to-kubelet-signer). The day-2 section is present only when a
deployed cluster's kubeconfig is found; pre-deploy runs are unaffected.
Code 64 (UsageError) covers problems with the flags on the
command line: an unknown flag, or a combination that is individually valid
but not sensible together — `--target`/`--only` without `--confirm-cluster`,
`--dry-run` combined with a `--skip-*` flag. Rule of thumb: if the fix is
"edit your YAML", it's ConfigError; if the fix is "change your command
line", it's UsageError.

## Examples

Run the next step only on success:

```sh
okdctl deploy && kubectl apply -f manifests/
```

Branch on specific failure categories:

```sh
okdctl deploy
rc=$?
case $rc in
  0)   echo "deploy succeeded" ;;
  2|65|66) echo "fix your config or pull secret, then retry" ;;
  3)   echo "network unreachable — check DNS and firewall" ;;
  4)   echo "cluster error — inspect oc logs" ;;
  5|71) echo "auth or privilege problem" ;;
  130) echo "interrupted" ;;
  143) echo "terminated by signal" ;;
  *)   echo "unexpected error (exit $rc)" ;;
esac
```

Detect and skip on interruption in CI:

```sh
okdctl deploy || { [ $? -eq 130 ] && exit 0 || exit 1; }
```

Alert on drift without failing a cron job:

```sh
okdctl plan
rc=$?
case $rc in
  0) echo "no drift" ;;
  7) echo "drift found — run 'okdctl deploy' to reconcile" | mail -s "okdctl drift" ops@example.com ;;
  *) echo "plan itself failed (exit $rc)"; exit 1 ;;
esac
```

## Source anchor

The code-to-error-type mapping lives in `internal/cli/root.go` (`exitCodeFor`
and `signalExitCode`). The package-doc comment at the top of that file is the
code-side anchor for this table.
