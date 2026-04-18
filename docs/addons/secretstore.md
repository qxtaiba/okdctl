# 1password secret store addon

Bootstraps 1Password Connect credentials into the cluster as Kubernetes
Secrets. The secrets land in the `external-secrets` namespace, ready for
the 1Password Connect server or external-secrets operator to consume.

Not enabled by default. Enable in the wizard or set
`addons.secretstore.enabled: true` in `okdctl.yaml`.

This addon creates secrets only. It does not install the 1Password Connect
server or the external-secrets operator.

## when to use

Use this addon when your workloads retrieve secrets from 1Password via
the Connect server and you want those bootstrap credentials placed in the
cluster automatically during post-install. It is a convenience layer over
`oc create secret` — it handles sops decryption and retries.

Skip it if you manage 1Password Connect credentials through another
mechanism (Vault, Sealed Secrets, manual `oc create secret`).

## default settings

| key | default | notes |
|---|---|---|
| `secrets_dir` | `automation/config/secrets` | path to credential files; relative paths are resolved from the project root |

Source: `secretstore.go:17` (`defaultSecretsDir`), `secretstore.go:148-152`
(`DefaultSettings`).

## configuration

```yaml
addons:
  secretstore:
    enabled: true
    settings:
      secrets_dir: automation/config/secrets
```

Place two files inside `secrets_dir`:

| file | k8s Secret name | data key |
|---|---|---|
| `1password-credentials.json` | `onepassword-connect-credentials` | `credentials_base64` |
| `1password-token.txt` | `onepassword-connect-token` | `token` |

Both files are optional: if a file is absent, the corresponding Secret is
skipped. If neither file exists, install warns and exits without error.

Files may be plaintext or sops-encrypted. The addon detects sops
encryption by scanning for a `"sops"` JSON key or a `sops_version=`
marker. Decryption uses `sops -d`; the age key must be at
`~/.config/sops/age/keys.txt`.

### obtaining the credential files

1. Download `1password-credentials.json` from **Settings > Automation**
   in 1password.com.
2. Create a Connect token and write it to `1password-token.txt`:
   ```sh
   echo -n 'YOUR_TOKEN' > automation/config/secrets/1password-token.txt
   ```
3. Copy the credentials file into the secrets directory.
4. Optionally encrypt with sops: `sops -e -i <file>`

## common failure modes

**Neither file present.** Install skips with a warning that lists the
setup steps above. Not an error — run `okdctl addon install secretstore`
again after placing the files.

**sops-encrypted files but sops not installed.** If the addon detects
sops-encrypted content but `sops` is not in `$PATH`, install fails with
an explicit error. Install `sops` (e.g. `brew install sops`) and retry.

**sops decryption fails.** The addon reports "sops decryption failed" and
suggests checking `~/.config/sops/age/keys.txt`. Causes include: wrong
age key, key file missing, file encrypted for a different recipient.

**Secret apply fails.** `oc apply -f -` errors are surfaced directly.
Typical causes: `oc` not logged in, `external-secrets` namespace creation
failed (RBAC issue).

**Verify finds Secret missing.** Verify checks each Secret only if the
corresponding source file exists. If a file was present at install time
but the Secret is gone (e.g., namespace was deleted), Verify returns an
error for that Secret.

## uninstall behaviour

`okdctl addon uninstall secretstore` deletes both Secrets:

- `oc delete secret onepassword-connect-credentials -n external-secrets`
- `oc delete secret onepassword-connect-token -n external-secrets`

Both deletions are warn-on-error; the command returns success regardless.
The `external-secrets` namespace is not deleted. The source files in
`secrets_dir` are not touched.

`Manager.Uninstall` blocks the operation if any other enabled addon
declares a dependency on `secretstore`.
