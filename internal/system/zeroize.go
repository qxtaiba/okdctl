package system

// ZeroBytes overwrites every byte in b with 0; use it (typically via defer)
// to bound a secret-bearing buffer's lifetime in process memory. Kept
// alongside internal/credentials.ProxmoxCredentials.Zeroize so every
// credential-zeroize call site shares one vocabulary instead of inlining
// clear().
func ZeroBytes(b []byte) {
	clear(b)
}
