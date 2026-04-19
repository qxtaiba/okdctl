# secret store addon

Bootstraps ESO (External Secrets Operator) provider credentials into the
cluster as Kubernetes Secrets, and applies an ESO `SecretStore` CRD that
configures the operator to use those credentials. Supports multiple
backends via the `provider` setting.

Not enabled by default. Enable in the wizard or set
`addons.secretstore.enabled: true` in `okdctl.yaml`.

This addon creates auth Secrets and the ESO SecretStore CRD only. It does
not install the ESO operator itself.

## when to use

Use this addon when your workloads pull secrets from an external backend
(1Password, Vault, etc.) via ESO and you want the bootstrap credentials
and the SecretStore CRD placed in the cluster automatically during
post-install.

## default settings

| key | default | notes |
|---|---|---|
| `provider` | `onepassword` | ESO backend: `onepassword`, `vault`, `bitwarden` |
| `secrets_dir` | `automation/config/secrets` | directory containing credential files; relative paths are resolved from the project root |
| `onepassword_vaults` | `homelab=1` | CSV of `name=priority` pairs used by the 1Password Connect SecretStore CRD (onepassword provider only) |
| `vault_path` | `secret` | Vault KV mount path (vault provider only) |
| `vault_version` | `v2` | Vault KV engine version: `v1` or `v2` (vault provider only) |
| `bitwarden_api_url` | `https://api.bitwarden.com` | Bitwarden API URL; override for self-hosted Vaultwarden (bitwarden provider only) |
| `bitwarden_identity_url` | `https://identity.bitwarden.com` | Bitwarden identity URL; override for self-hosted Vaultwarden (bitwarden provider only) |
| `bitwarden_sdk_server_url` | `https://bitwarden-sdk-server.external-secrets.svc.cluster.local:9998` | In-cluster bitwarden-sdk-server sidecar URL (bitwarden provider only) |

Source: `secretstore.go:19-30` (constants and setting keys),
`secretstore.go:DefaultSettings`.

## providers

### onepassword (default)

Reads `1password-credentials.json` and `1password-token.txt` from
`secrets_dir`. Creates two Opaque Secrets and an ESO SecretStore CRD
pointing at the 1Password Connect server.

Additional settings:

| key | default | notes |
|---|---|---|
| `onepassword_connect_host` | `http://onepassword-connect:8080` | URL of the 1Password Connect server |
| `onepassword_vaults` | `homelab=1` | CSV of `name=priority` pairs written into `spec.provider.onepassword.vaults`. Most homelab users have one vault and can leave this at the default or set `"myvault=1"`. Multi-vault: `"homelab=1,shared=2"`. A structured editor is tracked for a follow-up roadmap item. |

```yaml
addons:
  secretstore:
    enabled: true
    settings:
      provider: onepassword
      secrets_dir: automation/config/secrets
      onepassword_vaults: "homelab=1"
```

Files required in `secrets_dir`:

| file | k8s Secret name | data key |
|---|---|---|
| `1password-credentials.json` | `onepassword-connect-credentials` | `credentials_base64` |
| `1password-token.txt` | `onepassword-connect-token` | `token` |

Both files are optional: if a file is absent the corresponding Secret is
skipped. If neither file exists, install warns and exits without error.

#### obtaining the credential files

1. Download `1password-credentials.json` from **Settings > Automation**
   in 1password.com.
2. Create a Connect token and write it to `1password-token.txt`:
   ```sh
   echo -n 'YOUR_TOKEN' > automation/config/secrets/1password-token.txt
   ```
3. Copy the credentials file into the secrets directory.
4. Optionally encrypt with sops: `sops -e -i <file>`

### vault

Reads `vault-token.txt` from `secrets_dir`. Creates a `vault-token` Opaque
Secret and an ESO SecretStore CRD configured for Vault token auth.

Required settings:

| key | notes |
|---|---|
| `vault_server` | Vault server URL, e.g. `https://vault.example.com` |

```yaml
addons:
  secretstore:
    enabled: true
    settings:
      provider: vault
      vault_server: https://vault.example.com
      vault_path: secret
      vault_version: v2
      secrets_dir: automation/config/secrets
```

Place your Vault token in `secrets_dir/vault-token.txt` (plaintext or
sops-encrypted).

### bitwarden

Reads `bitwarden-token.txt` from `secrets_dir`. Creates a
`bitwarden-access-token` Opaque Secret and an ESO SecretStore CRD
configured for Bitwarden Secrets Manager (works against the SaaS at
`api.bitwarden.com` or against a self-hosted Vaultwarden instance —
point the three URL settings at your Vaultwarden endpoints).

Required settings:

| key | notes |
|---|---|
| `bitwarden_organization_id` | Bitwarden organization UUID |
| `bitwarden_project_id` | Bitwarden project UUID |

Optional URL overrides (see the defaults table at the top of this
page):

- `bitwarden_api_url`
- `bitwarden_identity_url`
- `bitwarden_sdk_server_url`

```yaml
addons:
  secretstore:
    enabled: true
    settings:
      provider: bitwarden
      secrets_dir: automation/config/secrets
      bitwarden_organization_id: 00000000-0000-0000-0000-000000000000
      bitwarden_project_id: 00000000-0000-0000-0000-000000000000
      # Override for self-hosted Vaultwarden:
      # bitwarden_api_url: https://vaultwarden.example.com/api
      # bitwarden_identity_url: https://vaultwarden.example.com/identity
```

Place your machine-account access token in
`secrets_dir/bitwarden-token.txt` (plaintext or sops-encrypted).

Note: ESO's Bitwarden provider requires an in-cluster
`bitwarden-sdk-server` sidecar. This addon does **not** deploy the
sidecar — deploy it separately (ESO's Helm chart ships one; see ESO
docs) and point `bitwarden_sdk_server_url` at it if the default
in-cluster URL doesn't match your layout.

## files may be sops-encrypted

All credential files (any provider) may be plaintext or sops-encrypted.
The addon detects sops encryption by scanning for a `"sops"` JSON key or a
`sops_version=` marker. Decryption uses `sops -d`; the age key must be at
`~/.config/sops/age/keys.txt`.

## resources created

Install applies, in order: auth Opaque Secret(s), then an ESO
`SecretStore` CRD named `okdctl-secretstore` in the `external-secrets`
namespace.

## common failure modes

**Required files absent.** Install skips with a warning listing the setup
steps. Not an error — run `okdctl addon install secretstore` again after
placing the files.

**sops-encrypted files but sops not installed.** Install fails with an
explicit error. Install `sops` (e.g. `brew install sops`) and retry.

**sops decryption fails.** The addon reports "sops decryption failed" and
suggests checking `~/.config/sops/age/keys.txt`. Causes include: wrong age
key, key file missing, file encrypted for a different recipient.

**Secret apply fails.** `oc apply -f -` errors are surfaced directly.
Typical causes: `oc` not logged in, `external-secrets` namespace creation
failed (RBAC).

**Verify finds Secret missing.** Verify checks each Secret by name. If a
Secret created at install time is gone (e.g. namespace was deleted),
Verify returns an error.

## uninstall behaviour

`okdctl addon uninstall secretstore` deletes the provider's auth Secrets
and the `okdctl-secretstore` SecretStore CRD. All deletions are
warn-on-error. The `external-secrets` namespace is not deleted. Source
files in `secrets_dir` are not touched.

`Manager.Uninstall` blocks the operation if any other enabled addon
declares a dependency on `secretstore`.
