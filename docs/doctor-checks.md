# Doctor checks reference

`okdctl doctor` runs 10 preflight checks against the local environment
before a deploy, plus 5 additional air-gap checks gated on `--airgap` or
`OKDCTL_AIRGAP=1`. The command is Linux-only (it reads `/etc/os-release`
and uses Linux syscalls). Checks run in the order listed below; the 10
base checks always execute and results are reported per-check. Exit code
is 0 when there are no `[fail]` results (`[warn]` is tolerated), 1
otherwise.

## Table of contents

1. [host os](#host-os)
2. [root check](#root-check)
3. [bin dir on path](#bin-dir-on-path)
4. [bin dir](#bin-dir)
5. [tools and packages](#tools-and-packages)
6. [sudo](#sudo)
7. [ssh public key](#ssh-public-key)
8. [pull secret](#pull-secret)
9. [disk space](#disk-space)
10. [host ports](#host-ports)

Air-gap checks (active when `--airgap` or `OKDCTL_AIRGAP=1`):

11. [airgap mirror reachable](#airgap-mirror-reachable)
12. [airgap release image digest pinned](#airgap-release-image-digest-pinned)
13. [airgap addon artifacts present](#airgap-addon-artifacts-present)
14. [airgap bootstrap oc present](#airgap-bootstrap-oc-present)
15. [airgap idms applied](#airgap-idms-applied)

---

## host os

**What it checks:** Reads `/etc/os-release` via `platform.Detect()` and
reports the OS ID, version, and family (rhel-family or debian-family). This
tells subsequent phases which package manager and service names to use.

**Fail message:**
```
cannot read /etc/os-release: <error>
```

**How to fix:** The check runs on Linux only. If `/etc/os-release` is absent
or unreadable, ensure you are on a supported Linux host. Non-Linux hosts
(macOS, Windows) cannot run `okdctl deploy`.

---

## root check

**What it checks:** Confirms the process is not running as `root` (UID 0).
`okdctl` uses `sudo` internally for privileged operations; running the entire
process as root bypasses that safety layer.

**Fail message:**
```
running as root; okdctl uses sudo internally
```

**How to fix:** Run `okdctl` as your regular unprivileged user. `main.preflight()`
already refuses root at startup, so this check should always be green in
normal use.

---

## bin dir on path

**What it checks:** Resolves the effective bin dir (OKDCTL_BIN_DIR env >
`deployment.bin_dir` in `okdctl.yaml` > `/usr/local/bin`) and verifies that
exact directory is present in `$PATH`. The setup phase installs `oc`,
`openshift-install`, and `terraform` there; they must be reachable after
installation. Membership is checked component-wise via `filepath.SplitList`,
so `/home/user/bin` does not false-positive against `/home/user/bin-archived`.
When the config file cannot be loaded, the detail is suffixed with
`(config unavailable; using default)` and a pass is demoted to warn.

**Warn message (default or `OKDCTL_BIN_DIR`-configured dir missing from `$PATH`):**
```
<dir> missing from $PATH; okdctl will prepend it at startup
```

**Fail message (config-file-only `deployment.bin_dir` missing from `$PATH`):**
```
<dir> missing from $PATH; add it to your shell profile (okdctl cannot auto-prepend a config-only dir)
```

**How to fix:** Add the resolved bin dir (replace `<your-bin-dir>` with the
value reported in the doctor output) to your shell profile:
```bash
echo 'export PATH="<your-bin-dir>:$PATH"' >> ~/.bashrc
source ~/.bashrc
```
`okdctl` auto-prepends the bin dir at startup only when the dir is the
default (`/usr/local/bin`) or set via `OKDCTL_BIN_DIR` — both are available
before config parsing. A dir set only via `deployment.bin_dir` in
`okdctl.yaml` must be added to `$PATH` manually; okdctl cannot auto-prepend
it because the config is not loaded at startup.

---

## bin dir

**What it checks:** Resolves the effective bin dir (`OKDCTL_BIN_DIR` env >
`deployment.bin_dir` in `okdctl.yaml` > `/usr/local/bin`) and verifies the
directory exists and is writable by the invoking user. Writability is probed
by creating and immediately removing a temporary file; missing directories
are reported separately. When the config file cannot be loaded the detail
text is suffixed with `(config unavailable; using default)` so a malformed
YAML does not hide behind a green pass row.

**Pass message:**
```
<dir> writable
```

**Warn message (default `/usr/local/bin` — either missing or not writable):**
```
/usr/local/bin does not exist; setup will create it as root via sudo
/usr/local/bin not writable by invoking user; setup will install as root via sudo
```

**Fail message (user-configured dir — either missing or not writable):**
```
<dir> does not exist; create it first (e.g. mkdir -p)
<dir> not writable by invoking user; setup runs under sudo so binaries will be root-owned — chown to your user if you want to manage them later
```

**How to fix:**

For the default `/usr/local/bin` a `[warn]` is expected and normal — `okdctl deploy`
escalates via sudo. For a user-configured `deployment.bin_dir`, ensure the directory
exists and is owned (or group-writable) by the invoking user:
```bash
mkdir -p ~/bin
# in okdctl.yaml:
# deployment:
#   bin_dir: /home/youruser/bin   # or ~/bin — tilde is expanded
```

---

## tools and packages

**What it checks:** Probes for three categories of binaries using `exec.LookPath`:

- **Host tools** (`curl`, `ssh`, `git`) — must already be installed; missing = `[fail]`.
- **Installable CLIs** (`oc`, `openshift-install`, `terraform`) — downloaded by
  setup into the configured bin dir (see [bin dir](#bin-dir); defaults to
  `/usr/local/bin`); missing = `[warn]`.
- **System packages** (`coreos-installer`, `haproxy`, `dnsmasq`, `apache`/`httpd`/`apache2`)
  — installed by setup via `dnf`/`apt`; missing = `[warn]`.

**Fail message (per missing host tool):**
```
missing; required before anything else will work
```

**Warn message (per missing installable CLI or system package):**
```
will be downloaded during setup
will be installed via package manager
```

**How to fix:**

For `[fail]` items install the host tools with your system package manager:
```bash
# rhel-family
sudo dnf install -y curl openssh git

# debian-family
sudo apt-get install -y curl openssh-client git
```

`[warn]` items are handled automatically by `okdctl deploy`; no manual action
is required unless setup fails.

---

## sudo

**What it checks:** Verifies that `sudo` is installed and that the current
user can escalate without a password prompt (NOPASSWD). A 2-second timeout
guards the probe.

**Fail message:**
```
sudo not installed
```

**Warn message:**
```
sudo requires a password; deploy will prompt
```

**How to fix:**

If `sudo` is not installed:
```bash
# rhel-family
sudo dnf install -y sudo

# debian-family
sudo apt-get install -y sudo
```

To enable passwordless sudo for your user, add a sudoers entry:
```bash
echo "$USER ALL=(ALL) NOPASSWD:ALL" | sudo tee /etc/sudoers.d/$USER
sudo chmod 0440 /etc/sudoers.d/$USER
```

Password-prompted sudo is only a `[warn]` — deploy will succeed, but long
install steps may stall waiting for the prompt.

---

## ssh public key

**What it checks:** Looks for a default SSH public key in `~/.ssh/` by
checking three candidates in order: `id_ed25519.pub`, `id_rsa.pub`,
`id_ecdsa.pub`. The key is injected into VM ignition configs so you can
SSH into cluster nodes.

**Warn messages:**
```
cannot resolve home directory
no default ssh public key found; you will need to specify one in the wizard
```

**How to fix:**

Generate an Ed25519 key pair if none exists:
```bash
ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -N ""
```

If your key is in a non-default path, specify it in the wizard when
`okdctl deploy` prompts for the SSH public key path.

---

## pull secret

**What it checks:** Reads the config file (default `okdctl.yaml`) to find the
`files.pull_secret` path, then validates that the file exists and contains valid
JSON with a non-empty `auths` map.

**Warn message (no config yet):**
```
no config yet at okdctl.yaml; run 'okdctl deploy' to set the pull secret path in the wizard
```

**Fail messages:**
```
cannot stat config: <error>
cannot load config: <error>
files.pull_secret not set in okdctl.yaml; run 'okdctl deploy' to configure
not found at <path> (download from https://console.redhat.com/openshift/install/pull-secret)
invalid json: <error>
missing or malformed 'auths' field: not a valid okd pull secret
'auths' is empty: pull secret has no registry entries
```

**How to fix:**

OKD does not require a Red Hat account. A minimal dummy pull secret works for
all community OKD installs:
```json
{"auths":{"fake":{"auth":"aWQ6cGFzcwo="}}}
```
Save it as `~/pull-secret.json` (the wizard default). If you need a real pull
secret for a private registry, download it from
`https://console.redhat.com/openshift/install/pull-secret` and save it to the
path configured in `okdctl.yaml` under `files.pull_secret`.

---

## disk space

**What it checks:** Uses `syscall.Statfs` on the home directory to compute free
space. At least 20 GB must be available. The deploy process downloads OKD tools,
builds custom CoreOS ISOs, and holds Terraform state under `~/okd-install`.

**Fail message:**
```
<N> gb free in <home> (need at least 20 gb)
```

**Warn messages (stat errors):**
```
cannot resolve user home
statfs failed: <error>
statfs returned a non-positive block size
```

**How to fix:**

Free at least 20 GB in your home directory before deploying. Typical space
consumers to clean up: Docker/Podman image caches, old VM disk images, and
previous `~/okd-install` runs from failed deployments.

---

## host ports

**What it checks:** Attempts a TCP connect to `127.0.0.1` on each of the
following ports: `53`, `80`, `443`, `6443`, `22623`, `8080`. A successful
connect means another service is already bound there; deploy services
(`haproxy`, `dnsmasq`, `apache`) will conflict.

**Warn message:**
```
in use: <ports> (stop the conflicting service before deploy)
```

**How to fix:**

Identify and stop the conflicting service for each busy port:
```bash
sudo ss -tlnp | grep -E ':(53|80|443|6443|22623|8080) '
```

Common culprits and fixes:

| Port | Common culprit | Fix |
|------|---------------|-----|
| 53 | `systemd-resolved` | `sudo systemctl stop systemd-resolved` and set `DNSStubListener=no` in `/etc/systemd/resolved.conf` |
| 80 / 443 | Existing web server | `sudo systemctl stop httpd apache2 nginx` |
| 8080 | Any HTTP proxy or app | Stop or reconfigure the service |
| 6443 | Existing k8s API server | Stop the conflicting cluster |
| 22623 | Another OKD install | Stop or destroy the existing cluster first |

If the service on the port is intentional, stop or reconfigure it before
running `okdctl deploy` — okdctl always binds to the ports listed above.

---

## airgap mirror reachable

**What it checks:** Resolves every HTTPS blob in the active FetchPlan
(helm, sops, yq, bootstrap oc, CoreOS stream metadata) through
`OKDCTL_MIRROR_BASE` and sends an HTTP HEAD to each resolved URL. All
must return HTTP 200 or 302.

**Fail message (per blob):**
```
HTTP <N> at <url>; stage via fetch-blobs.sh
<url> — <err>; stage via fetch-blobs.sh
```

**How to fix:** Re-run `./fetch-blobs.sh` from your airgap plan output
directory to stage missing blobs onto the mirror host. Ensure
`OKDCTL_MIRROR_BASE` (or `deployment.mirror_base` in `okdctl.yaml`)
points at the correct mirror URL.

---

## airgap release image digest pinned

**What it checks:** Probes `quay.io/okd/scos-release:<version>` in the
mirror via OCI manifest HEAD (`/v2/<name>/manifests/<tag>`). Reports the
image as reachable, then emits a `[warn]` noting that cosign verification
is not yet available for OKD release images (tracked at
[okd-project/okd#2092](https://github.com/okd-project/okd/issues/2092)).
Digest pinning via the OCI manifest response is the available verification
mechanism today.

**Warn message (reachable):**
```
<resolved-ref> reachable — digest-pinning only; cosign verification pending okd-project/okd#2092
```

**Fail message:**
```
HTTP <N> from <resolved-ref>; push quay.io/okd/scos-release:<version> to your mirror via run-oc-mirror.sh
cannot reach <resolved-ref>: <err>; push quay.io/okd/scos-release:<version> to your mirror, then re-run run-oc-mirror.sh
```

**How to fix:** Run `./run-oc-mirror.sh` from your airgap plan output
directory to mirror the OKD release image. Ensure `distribution.version`
is set in `okdctl.yaml`.

---

## airgap addon artifacts present

**What it checks:** For every `MirrorableAddon` (currently: flux), runs
`helm template` to discover transitive container images declared in the
addon's charts, then probes each image ref against the mirror via OCI
manifest HEAD.

**Warn message (helm absent or chart unreachable):**
```
helm template failed (<err>); ensure helm is on PATH and charts are reachable upstream
```

**Fail message (per image):**
```
HTTP 404 at <resolved-ref>; add to isc.yaml additionalImages and re-run oc-mirror
HTTP <N> at <resolved-ref>; re-run ./run-oc-mirror.sh
<resolved-ref> — <err>; re-run ./run-oc-mirror.sh
```

**How to fix:** Re-run `./run-oc-mirror.sh`. If an image is not covered
by the `ImageSetConfiguration`, add it to the `additionalImages:` section
in `isc.yaml` and re-run oc-mirror.

---

## airgap bootstrap oc present

**What it checks:** Confirms `oc` is on `$PATH`. The setup phase uses
`oc adm release extract --tools` to obtain the OKD binaries; a missing
`oc` blocks setup.

**Pass message:**
```
oc found on $PATH
```

**Fail message:**
```
oc not found on $PATH; fetch from https://mirror.openshift.com/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz and extract to a directory on $PATH
```

**How to fix:** Download the bootstrap `oc` client, extract it, and
place the `oc` binary in a directory on `$PATH`. In an air-gap
environment fetch from your mirror at
`<OKDCTL_MIRROR_BASE>/openshift-mirror/pub/openshift-v4/clients/oc/latest/linux/oc.tar.gz`.

---

## airgap idms applied

**What it checks:** Probes the connected cluster for
`ImageDigestMirrorSet` and `ImageTagMirrorSet` resources via `oc get`.
These resources redirect image pulls from upstream registries to the
air-gap mirror at cluster runtime. The check is skipped pre-deploy
(when no kubeconfig is present).

**Warn message (pre-deploy / no cluster):**
```
cluster not reachable (no kubeconfig); run this check after okdctl deploy
```

**Pass message:**
```
ImageDigestMirrorSet / ImageTagMirrorSet applied on cluster
```

**Fail message:**
```
no ImageDigestMirrorSet or ImageTagMirrorSet found; apply oc-mirror output: oc apply -f oc-mirror-workspace/results-*/
```

**How to fix:** Apply the IDMS/ITMS manifests produced by `oc-mirror --v2`:
```bash
oc apply -f oc-mirror-workspace/results-*/
```
Then re-run `okdctl doctor --airgap` to confirm the resources are present.
