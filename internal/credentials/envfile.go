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

var errEnvFileAlreadyLoaded = errors.New("env file already loaded with different path")

// ErrEnvFileUnknownKey reports a key outside the PROXMOX_VE_API_TOKEN and
// PROXMOX_VE_INSECURE allowlist, so a typo or a smuggled variable fails
// loudly instead of silently reaching the process environment.
var ErrEnvFileUnknownKey = errors.New("env file contains an unrecognized key")

var envFileAllowedKeys = map[string]bool{
	envProxmoxAPIToken: true,
	envProxmoxInsecure: true,
}

// loadOnce guards LoadEnvFile: env mutation is not goroutine-safe, and a
// second call with a different path must fail rather than reuse the first.
var (
	loadOnce   sync.Once
	loadErr    error
	loadedPath string
)

// EnvFilePath derives the sidecar path from a config path, okdctl.yaml
// becoming okdctl.env.
func EnvFilePath(configPath string) string {
	if strings.HasSuffix(configPath, ".yaml") {
		return strings.TrimSuffix(configPath, ".yaml") + ".env"
	}
	if strings.HasSuffix(configPath, ".yml") {
		return strings.TrimSuffix(configPath, ".yml") + ".env"
	}
	return configPath + ".env"
}

// WriteEnvFile persists the token and TLS flag as KEY=VALUE lines at mode
// 0600, refusing a symlinked path and zeroing its buffer after the write.
func WriteEnvFile(path string, creds *ProxmoxCredentials) error {
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

	return writeEnvFileBody(creds, func(data []byte) error {
		return system.AtomicWrite(path, data, 0o600)
	})
}

func writeEnvFileBody(creds *ProxmoxCredentials, write func([]byte) error) error {
	v := creds.APIToken
	quoted := len(v) >= 2 && ((v[0] == '\'' && v[len(v)-1] == '\'') ||
		(v[0] == '"' && v[len(v)-1] == '"'))
	if bytes.ContainsAny(v, "\r\n\x00") || !bytes.Equal(v, bytes.TrimSpace(v)) ||
		quoted || len(envProxmoxAPIToken)+len(v)+2 >= bufio.MaxScanTokenSize {
		return &errtypes.AuthError{Msg: "encode env file: " + envProxmoxAPIToken + " cannot be represented losslessly"}
	}
	data := buildEnvFileBody(creds)
	defer clear(data)
	return write(data)
}

// buildEnvFileBody preallocates the exact final size so a mid-write grow
// cannot orphan an un-zeroed copy of the token in an old backing array.
func buildEnvFileBody(creds *ProxmoxCredentials) []byte {
	const header = "# Proxmox credentials (managed by okdctl)\n" +
		"# This file has restricted permissions (0600). Do not commit to git.\n"
	const insecureLine = envProxmoxInsecure + "=true\n"

	n := len(header)
	if len(creds.APIToken) > 0 {
		n += len(envProxmoxAPIToken) + 1 + len(creds.APIToken) + 1
	}
	if creds.Insecure {
		n += len(insecureLine)
	}

	buf := bytes.NewBuffer(make([]byte, 0, n))
	buf.WriteString(header)
	if len(creds.APIToken) > 0 {
		buf.WriteString(envProxmoxAPIToken + "=")
		buf.Write(creds.APIToken)
		buf.WriteByte('\n')
	}
	if creds.Insecure {
		buf.WriteString(insecureLine)
	}
	return buf.Bytes()
}

// LoadEnvFile loads path into the process environment once per process;
// already-set variables win and a missing file is not an error. A non-nil
// return means nothing was loaded.
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

func loadEnvFileOnce(path string) error {
	pairs, err := readEnvFile(path)
	if err != nil {
		return err
	}
	for k, v := range pairs {
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

// ReadEnvFile returns the token and TLS flag the sidecar at path holds,
// empty when the file is missing; the process environment is never consulted.
func ReadEnvFile(path string) (*ProxmoxCredentials, error) {
	pairs, err := readEnvFile(path)
	if err != nil {
		return nil, err
	}
	creds := &ProxmoxCredentials{Insecure: pairs[envProxmoxInsecure] == "true"}
	if token := pairs[envProxmoxAPIToken]; token != "" {
		creds.APIToken = []byte(token)
	}
	return creds, nil
}

// readEnvFile opens O_NOFOLLOW so the permission check and the read bind to
// one inode, closing the stat-then-open TOCTOU window.
func readEnvFile(path string) (map[string]string, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("open env file %s", path), Err: err}
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("stat env file %s", path), Err: err}
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		return nil, &errtypes.AuthError{
			Msg:  fmt.Sprintf(".env file has insecure permissions %#o; run 'chmod 600 <path>' to fix", perm),
			Path: path,
			Err:  os.ErrPermission,
		}
	}

	pairs, err := parseDotEnv(f)
	if err != nil {
		return nil, &errtypes.ConfigError{Msg: fmt.Sprintf("parse env file %s", path), Err: err}
	}
	if err := rejectUnknownEnvKeys(path, pairs); err != nil {
		return nil, err
	}
	return pairs, nil
}

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
		Msg: fmt.Sprintf("env file %s contains unrecognized keys %v; only %s and %s are permitted",
			path, rejected, envProxmoxAPIToken, envProxmoxInsecure),
		Err: ErrEnvFileUnknownKey,
	}
}

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
