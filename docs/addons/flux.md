# flux gitops addon

Bootstraps [FluxCD](https://fluxcd.io) into an OKD cluster using the
controlplane.io Flux Operator. Flux watches a Git repository and
reconciles cluster state continuously.

Not enabled by default. Enable in the wizard or set
`addons.flux.enabled: true` in `okdctl.yaml`.

## when to use

Use this addon when you want GitOps-driven delivery: manifests live in a
Git repository, and the cluster self-heals toward that state. It is not a
replacement for `okdctl deploy` — Flux manages workloads after the cluster
is up.

Skip it if your cluster state is managed externally (ArgoCD, manual `oc
apply`) or if you have no Git repository to point it at.

## default settings

| key | default | notes |
|---|---|---|
| `repository` | _(required)_ | Git URL; no default |
| `branch` | `main` | branch to sync |
| `path` | `kubernetes/clusters/production` | path within repo |
| `controller_timeout` | `300` (seconds) | how long to wait for controllers |
| `git_sync_timeout` | `180` (seconds) | how long to wait for first git sync |

Sources: `flux.go:223-231` (`DefaultSettings`), `flux.go:22-25` (duration
constants).

`repository` accepts `ssh://`, `https://`, `git://`, and `git@host:path`
forms. Branch names cannot contain whitespace (spaces or tabs). Path is
restricted to `[a-zA-Z0-9/_.-]`.

## configuration

Set in `okdctl.yaml` under `addons.flux.settings`:

```yaml
addons:
  flux:
    enabled: true
    settings:
      repository: "git@github.com:org/repo.git"
      branch: main
      path: kubernetes/clusters/production
```

## deploy key

Flux authenticates to the Git repository using an SSH deploy key at
`~/.ssh/flux-deploy-key`. Generate it before running `deploy`:

```sh
ssh-keygen -t ed25519 -f ~/.ssh/flux-deploy-key -N ''
```

Then add the public key (`~/.ssh/flux-deploy-key.pub`) as a read-only
deploy key on your Git host. The public half is optional — Flux only
requires the private key and `known_hosts`. Install reads the key,
runs `ssh-keyscan` against the resolved git host, and stores the result
as a `flux-system` Secret in the `flux-system` namespace.

If the deploy key file is missing, install fails immediately with the
path and the `ssh-keygen` command to fix it.

## common failure modes

**`helm` not found.** The addon requires `helm` in `$PATH`. Install
returns an error before doing anything if `helm` is absent.

**deploy key missing.** If `~/.ssh/flux-deploy-key` does not exist,
install fails with a message showing the expected path and the
`ssh-keygen` command.

**flux-operator not ready within 120s.** After the operator chart
installs, install runs `oc wait --for=condition=available
deployment/flux-operator --timeout=120s`. If the operator pod does not
become available in that window, install fails. Check pod events in the
`flux-system` namespace.

**controllers not ready within 5 minutes.** After the instance chart
installs, install polls all deployments labelled
`app.kubernetes.io/part-of=flux` in `flux-system`. If any have zero
available replicas after `controller_timeout` seconds (default 300),
install fails.

**git sync not ready within 3 minutes.** This failure is non-fatal.
Install warns and continues; the cluster will reconcile once the
repository is reachable. Debug with:

```sh
oc get gitrepository -n flux-system -o yaml
```

Common causes: deploy key not added to the git host, repository URL
typo, network policy blocking egress.

**Verify reports source-controller unhealthy.** If `Verify` cannot query
the `source-controller` deployment, it warns and continues. If the query
succeeds and reports zero ready replicas, `Verify` returns a fatal
"source-controller has no ready replicas" error. Flux cannot sync
manifests in either state.

## uninstall behaviour

`okdctl addon uninstall flux` runs:

1. `helm uninstall flux-instance --namespace flux-system`
2. `helm uninstall flux-operator --namespace flux-system`
3. `oc delete ns flux-system`

Each step is warn-on-error; the command returns success regardless. The
deploy key Secret is removed with the namespace. The SSH key files at
`~/.ssh/flux-deploy-key` are not touched.

`Manager.Uninstall` blocks the operation if any other enabled addon
declares a dependency on `flux`.
