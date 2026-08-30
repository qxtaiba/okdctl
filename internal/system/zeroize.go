package system

// ZeroBytes overwrites every byte in b with 0, typically via defer, to bound
// a secret buffer's lifetime in memory. Kept alongside
// credentials.ProxmoxCredentials.Zeroize so every credential-zeroize call
// site shares one vocabulary.
func ZeroBytes(b []byte) {
	clear(b)
}
