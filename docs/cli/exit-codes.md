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
| 64   | EX_USAGE     | unknown flag (cobra flag-parse failure)                      |
| 65   | EX_DATAERR   | pull secret file exists but is not valid JSON                |
| 66   | EX_NOINPUT   | configuration file not found on disk                         |
| 71   | EX_OSERR     | sudo not found on PATH (requires root operation)             |
| 130  | —            | interrupted by SIGINT (Ctrl-C)                               |
| 143  | —            | terminated by SIGTERM                                        |

Codes 65, 66, and 71 are granular refinements within the broader categories
2 (config) and 5 (auth). A script that only checks for non-zero exit is
unaffected; a script that branches on code 2 or 5 should also handle 65/66/71.

Commands that do not require root (`status`, `config`, `kubeconfig`, and
others) exit with code 5 when invoked under `sudo` or as root — the binary
refuses with "do not run as root/sudo; this tool escalates internally".
Root-requiring commands (`deploy`, `destroy`, `cleanup`, `update-ingress`)
must be invoked as a regular user; the binary self-elevates via an internal
`sudo` re-exec so the privileged body runs as euid=0.

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

## Source anchor

The code-to-error-type mapping lives in `internal/cli/root.go` (`exitCodeFor`
and `signalExitCode`). The package-doc comment at the top of that file is the
code-side anchor for this table.
