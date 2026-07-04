package service

import (
	"os"
	"path/filepath"
)

// transferSidecarNFO moves/copies/links the .nfo sidecar alongside its media
// using the same transfer mode, so metadata follows the organized file.
func transferSidecarNFO(srcMedia, dstMedia string, mode TransferMode) error {
	src := nfoPath(srcMedia)
	dst := nfoPath(dstMedia)
	if src == dst {
		return nil
	}
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { // #nosec G301 -- sidecar media directories must remain readable by NAS/player users.
		return err
	}
	return transferFile(src, dst, mode)
}
