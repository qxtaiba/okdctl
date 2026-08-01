package credentials

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/qxtaiba/okdctl/internal/errtypes"
	"github.com/qxtaiba/okdctl/internal/system"
)

// errEnvFileAlreadyLoaded is wrapped inside the ConfigError LoadEnvFile
// returns when called a second time with a path different from the
// first call, so callers can errors.Is it instead of string-matching Msg.
var errEnvFileAlreadyLoaded = errors.New("env file already loaded with different path")

// ErrEnvFileUnknownKey is wrapped inside the ConfigError LoadEnvFile returns
// when the .env carries a key outside the PROXMOX_VE_* allowlist, so a typo or
// an attempt to smuggle TF_/KUBE/OC_ variables into every subprocess is a
// visible failure rather than a silent promotion into the process environment.
var ErrEnvFileUnknownKey = errors.New("env file contains an unrecognized key")

// envFileAllowedKeys is the exact set of keys promoted from the .env file into
// the process environment. It is an allowlist by design: keys under TF_, KUBE,
// PROXMOX_, HELM_, or OC_ prefixes would otherwise pass DefaultEnvAllowlist and
// reach every subprocess, including terraform under the sudo re-exec.
var envFileAllowedKeys = map[string]bool{
	envProxmoxEndpoint: true,
	envProxmoxUsername: true,
	envProxmoxPassword: true,
	envProxmoxAPIToken: true,
	envProxmoxInsecure: true,
}

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
	} else if !errors.Is(err, os.ErrNotExist) {
		return &errtypes.AuthError{
			Msg: fmt.Sprintf("lstat env file path %q before write", path),
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
//
// The buffer is preallocated to the exact final size so it never reallocates
// after the first secret byte is written: a mid-write grow would orphan an
// un-zeroed copy of the password/token in the old backing array, which
// WriteEnvFile's clear() can no longer reach.
func buildEnvFileBody(creds *ProxmoxCredentials) []byte {
	const header = "# Proxmox credentials (managed by okdctl)\n" +
		"# This file has restricted permissions (0600). Do not commit to git.\n"

	n := len(header)
	if creds.Endpoint != "" {
		n += len(envProxmoxEndpoint) + 1 + len(creds.Endpoint) + 1
	}
	if creds.Username != "" {
		n += len(envProxmoxUsername) + 1 + len(creds.Username) + 1
	}
	if len(creds.Password) > 0 {
		n += len(envProxmoxPassword) + 1 + len(creds.Password) + 1
	}
	if len(creds.APIToken) > 0 {
		n += len(envProxmoxAPIToken) + 1 + len(creds.APIToken) + 1
	}
	if creds.Insecure {
		n += len(envProxmoxInsecure) + len("=true\n")
	}

	buf := bytes.NewBuffer(make([]byte, 0, n))
	buf.WriteString(header)

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
//
// A non-nil return always means credentials were not loaded. Callers must
// treat any error as fatal and abort the operation rather than proceeding
// with potentially missing credentials.
func LoadEnvFile(path string) error {
	loadOnce.Do(func() {
		loadedPath = path
		loadErr = loadEnvFileOnce(path)
	})
	if loadedPath != path {
		return &errtypes.ConfigError{
			Msg: fmt.Sprintf("LoadEnvFile already called with %q; cannot reload from %q", loadedPath, path),
			Err: errEnvFileAlreadyLoaded,
		}
	}
	return loadErr
}

// loadEnvFileOnce is not safe to call concurrently and must only be
// invoked via the LoadEnvFile sync.Once.
func loadEnvFileOnce(path string) error {
	// Refuse to load a .env that any other user can read. Proxmox tokens
	// end up in here — a world-readable file defeats the whole point of
	// moving secrets out of YAML. Open with O_NOFOLLOW first, then derive the
	// permission decision from f.Stat() and read the contents from that same
	// descriptor: binding the check and the read to one inode closes the
	// TOCTOU window an os.Stat-then-os.Open pair leaves open, and O_NOFOLLOW
	// refuses a symlinked path just as WriteEnvFile's Lstat does.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // missing .env is not an error
		}
		return &errtypes.ConfigError{Msg: fmt.Sprintf("open env file %s", path), Err: err}
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("stat env file %s", path), Err: err}
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return &errtypes.AuthError{
			Msg:  fmt.Sprintf(".env file has insecure permissions %#o; run 'chmod 600 <path>' to fix", perm),
			Path: path,
			Err:  os.ErrPermission,
		}
	}

	pairs, err := parseDotEnv(f)
	if err != nil {
		return &errtypes.ConfigError{Msg: fmt.Sprintf("parse env file %s", path), Err: err}
	}

	if err := rejectUnknownEnvKeys(path, pairs); err != nil {
		return err
	}

	// Shell environment takes precedence: set only keys that are absent.
	for k, v := range pairs {
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

// rejectUnknownEnvKeys fails when pairs carries any key outside
// envFileAllowedKeys, naming the offenders so a typo surfaces instead of being
// silently dropped. It sets nothing — a non-nil return leaves the environment
// untouched, matching LoadEnvFile's "error means nothing loaded" contract.
func rejectUnknownEnvKeys(path string, pairs map[string]string) error {
	var rejected []string
	for k := range pairs {
		if !envFileAllowedKeys[k] {
			rejected = append(rejected, k)
		}
	}
	if len(rejected) == 0 {
		return nil
	}
	slices.Sort(rejected)
	return &errtypes.ConfigError{
		Msg: fmt.Sprintf("env file %s contains unrecognized keys %v; only PROXMOX_VE_* credential keys are permitted",
			path, rejected),
		Err: ErrEnvFileUnknownKey,
	}
}

// parseDotEnv reads key=value pairs from r. Blank lines and lines whose first
// non-space character is '#' are skipped; lines without '=' return an error.
// Surrounding single or double quotes are stripped from values.
func parseDotEnv(r io.Reader) (map[string]string, error) {
	pairs := make(map[string]string)
	sc := bufio.NewScanner(r)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			return nil, fmt.Errorf("malformed line %d (no '=')", lineNum)
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
