package system

// ZeroBytes overwrites every byte in b with 0. Use it (typically via
// defer) to bound the lifetime of a secret-bearing buffer in process
// memory once the secret has been consumed. The method-bound equivalent
// for credentials is internal/credentials.ProxmoxCredentials.Zeroize.
func ZeroBytes(b []byte) {
	clear(b)
}
