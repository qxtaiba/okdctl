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

// errEnvFileAlreadyLoaded lets callers errors.Is the second-call failure
// instead of string-matching Msg.
var errEnvFileAlreadyLoaded = errors.New("env file already loaded with different path")

// ErrEnvFileUnknownKey is returned when the .env file contains a key outside
// the PROXMOX_VE_* allowlist, so a typo or a smuggled TF_/KUBE/OC_ variable
// fails loudly instead of silently reaching every subprocess.
var ErrEnvFileUnknownKey = errors.New("env file contains an unrecognized key")

// envFileAllowedKeys is an explicit allowlist: without it, TF_/KUBE/PROXMOX_/
// HELM_/OC_-prefixed keys would pass DefaultEnvAllowlist into every
// subprocess, including terraform under the sudo re-exec.
var envFileAllowedKeys = map[string]bool{
	envProxmoxEndpoint: true,
	envProxmoxUsername: true,
	envProxmoxPassword: true,
	envProxmoxAPIToken: true,
	envProxmoxInsecure: true,
}

// loadOnce guards against concurrent/repeated LoadEnvFile calls — env
// mutation isn't goroutine-safe, and a second call with a different path
// silently reuses the first result.
var (
	loadOnce   sync.Once
	loadErr    error
	loadedPath string
)

// EnvFilePath derives the .env path from a config path (e.g. "okdctl.yaml" becomes "okdctl.env").
func EnvFilePath(configPath string) string {
	if strings.HasSuffix(configPath, ".yaml") {
		return strings.TrimSuffix(configPath, ".yaml") + ".env"
	}
	if strings.HasSuffix(configPath, ".yml") {
		return strings.TrimSuffix(configPath, ".yml") + ".env"
	}
	return configPath + ".env"
}

// WriteEnvFile persists credentials in KEY=VALUE format with 0600
// permissions. The buffer is zeroed after write so credential residue
// doesn't outlive the call.
func WriteEnvFile(path string, creds *ProxmoxCredentials) error {
	// Refuse symlinks: a pre-planted symlink would redirect credential bytes
	// to an attacker-chosen target; a missing file here is the normal
	// first-write case.
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
	clear(data)
	return err
}

// buildEnvFileBody serialises creds into KEY=VALUE format; the caller must
// clear() the returned slice. The buffer is preallocated to its exact final
// size so a mid-write grow can't orphan an un-zeroed secret copy in the old
// backing array.
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

// LoadEnvFile loads a .env file into the process environment once per
// process (guarded by sync.Once); already-set variables are not overwritten
// and a missing file is not an error. A non-nil return means nothing was
// loaded — callers must treat it as fatal.
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

// loadEnvFileOnce is not concurrency-safe; call only via LoadEnvFile's
// sync.Once.
func loadEnvFileOnce(path string) error {
	// O_NOFOLLOW binds the permission check and read to one inode, closing
	// the TOCTOU window an os.Stat-then-Open pair leaves open, and refuses a
	// symlinked .env just like WriteEnvFile's Lstat.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
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

	for k, v := range pairs {
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

// rejectUnknownEnvKeys names offending keys and, on error, leaves the
// environment untouched — matching LoadEnvFile's "error means nothing
// loaded" contract.
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

// parseDotEnv reads key=value pairs from r, skipping blank/comment lines and
// stripping surrounding quotes from values.
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
