## okdctl airgap plan

Emit oc-mirror ImageSetConfiguration and blob manifest for an air-gap deploy

### Synopsis

Generate the four artifacts needed to mirror an OKD release into a
disconnected environment:

  isc.yaml           mirror.openshift.io/v2alpha1 ImageSetConfiguration
  airgap.yaml        HTTPS blob list with pinned SHA256s
  run-oc-mirror.sh   wrapper script exporting OKD CI signature env vars
  fetch-blobs.sh     helper to stage blobs into a local directory

The --version flag selects the OKD release. --release-digest must supply
the quay.io/okd/scos-release image digest for that version (obtain it via
"oc adm release info quay.io/okd/scos-release:<version> --output=jsonpath='{.digest}'").

```
okdctl airgap plan [flags]
```

### Options

```
      --channel string          when set, emit graph-based ISC for this channel (e.g. stable-4.21)
  -h, --help                    help for plan
      --out-dir string          directory to write artifacts into (created if absent) (default "airgap")
      --release-digest string   digest of quay.io/okd/scos-release:<version> (sha256:<hex>)
      --stream-json string      path to a local scos.json/fcos.json file (skips network fetch; useful offline)
      --version string          OKD version to plan for (e.g. 4.21.0-okd-scos.10)
```

### Options inherited from parent commands

```
  -c, --config string       configuration file (default "okdctl.yaml")
      --log-file string     write log output to this file in addition to stderr
      --log-format string   log output format (text, json) (default "text")
      --log-level string    log verbosity (debug, info, warn, error) (default "info")
  -q, --quiet               suppress info/warn logs (alias for --log-level=error)
  -v, --verbose             enable debug logging (alias for --log-level=debug)
```

### SEE ALSO

* [okdctl airgap](okdctl_airgap.md)	 - Tools for air-gap deployments

