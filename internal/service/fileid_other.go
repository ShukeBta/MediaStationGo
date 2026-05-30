//go:build !unix

package service

// fileIdentity is a no-op on platforms without inode semantics; dedup by
// hardlink identity is simply disabled there.
func fileIdentity(path string) (string, bool) {
	return "", false
}
