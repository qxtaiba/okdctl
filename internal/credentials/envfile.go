package credentials

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// loadOnce guards against concurrent or repeated calls to LoadEnvFile.
// The global process environment is shared mutable state: without this
// guard, a "getenv empty → setenv" sequence races against any other
// goroutine touching the same key. The .env file is intentionally a
// per-process artefact — callers that pass a different path on a second
// invocation silently get the first path's result, which is preferred
// over letting multiple sources mutate the environment.
var (
	loadOnce   sync.Once
	loadErr    error
	loadedPath string
)

// EnvFilePath derives the .env path from a config path
// (e.g. "okdctl.yaml" becomes "okdctl.env").
func EnvFilePath(configPath string) string {
	if strings.HasSuffix(configPath, ".yaml") {
		return strings.TrimSuffix(configPath, ".yaml") + ".env"
	}
	if strings.HasSuffix(configPath, ".yml") {
		return strings.TrimSuffix(configPath, ".yml") + ".env"
	}
	return configPath + ".env"
}

// WriteEnvFile persists credentials in KEY=VALUE format compatible with
// standard .env tooling, with 0600 permissions.
//
// Content is built via bytes.Buffer so the credential bytes are appended
// directly without creating an intermediate immutable string copy. The
// buffer is zeroed after AtomicWrite returns so the in-memory residue
// doesn't outlive the write.
func WriteEnvFile(path string, creds *ProxmoxCredentials) error {
	// Refuse symlinks: an attacker-planted symlink at the .env path would
	// redirect credential bytes to an attacker-chosen target. Lstat the
	// path before writing; a missing file is the normal first-write case.
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return &errtypes.AuthError{
				Msg: fmt.Sprintf("env file path %q is a symlink; refusing to write credentials", path),
				Err: os.ErrPermission,
			}
		}
	} else if !os.IsNotExist(err) {
		return &errtypes.AuthError{
			Msg: fmt.Sprintf("failed to lstat env file path %q before write", path),
			Err: err,
		}
	}

	data := buildEnvFileBody(creds)
	err := system.AtomicWrite(path, data, 0o600)
	// Zero the buffer's backing store so the credential bytes don't
	// linger after the file write completes.
	clear(data)
	return err
}

// buildEnvFileBody serialises creds into KEY=VALUE format. The caller owns
// the returned slice and must clear it after use.
func buildEnvFileBody(creds *ProxmoxCredentials) []byte {
	var buf bytes.Buffer
	buf.WriteString("# Proxmox credentials (managed by okdctl)\n")
	buf.WriteString("# This file has restricted permissions (0600). Do not commit to git.\n")

	if creds.Endpoint != "" {
		buf.WriteString(envProxmoxEndpoint + "=")
		buf.WriteString(creds.Endpoint)
		buf.WriteByte('\n')
	}
	if creds.Username != "" {
		buf.WriteString(envProxmoxUsername + "=")
		buf.WriteString(creds.Username)
		buf.WriteByte('\n')
	}
	if len(creds.Password) > 0 {
		buf.WriteString(envProxmoxPassword + "=")
		buf.Write(creds.Password)
		buf.WriteByte('\n')
	}
	if len(creds.APIToken) > 0 {
		buf.WriteString(envProxmoxAPIToken + "=")
		buf.Write(creds.APIToken)
		buf.WriteByte('\n')
	}
	if creds.Insecure {
		buf.WriteString(envProxmoxInsecure + "=true\n")
	}
	return buf.Bytes()
}

// LoadEnvFile loads a .env file into the process environment.
// Already-set variables are NOT overwritten (shell env takes precedence).
// Missing files are silently ignored.
//
// LoadEnvFile is guarded by a sync.Once: the underlying work runs at most
// once per process regardless of how many times (or with which paths) it
// is invoked. Subsequent calls return the first call's error. This is
// intentional — mutating os.Environ from multiple sources is a footgun,
// and the .env file is a per-process resource.
func LoadEnvFile(path string) error {
	loadOnce.Do(func() {
		loadedPath = path
		loadErr = loadEnvFileOnce(path)
	})
	if loadedPath != path {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("LoadEnvFile already called with %q; cannot reload from %q", loadedPath, path)}
	}
	return loadErr
}

// loadEnvFileOnce contains the actual load logic. It is not safe to call
// concurrently and must only be invoked via the LoadEnvFile sync.Once.
func loadEnvFileOnce(path string) error {
	// Refuse to load a .env that any other user can read. Proxmox tokens
	// end up in here — a world-readable file defeats the whole point of
	// moving secrets out of YAML. The permission check MUST happen before
	// the file is opened: once its contents are in our address space,
	// the whole point of this check is to detect a file that other
	// processes may already have been able to read.
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // missing .env is not an error
		}
		return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to stat env file %s", path), Err: err}
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return &errtypes.AuthError{
			Msg:  fmt.Sprintf(".env file has insecure permissions %#o; run 'chmod 600 <path>' to fix", perm),
			Path: path,
			Err:  os.ErrPermission,
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to open env file %s", path), Err: err}
	}
	defer f.Close() //nolint:errcheck // read-only open; close error is not actionable

	pairs, err := parseDotEnv(f)
	if err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("failed to parse env file %s", path), Err: err}
	}

	// Shell environment takes precedence: set only keys that are absent.
	for k, v := range pairs {
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

// parseDotEnv reads key=value pairs from r. Blank lines and lines whose first
// non-space character is '#' are skipped; lines without '=' return an error.
// Surrounding single or double quotes are stripped from values.
func parseDotEnv(r io.Reader) (map[string]string, error) {
	pairs := make(map[string]string)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			return nil, fmt.Errorf("malformed line (no '='): %q", line)
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		pairs[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return pairs, nil
}
