package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func organizeTransferExtension(source string, mode TransferMode) string {
	if mode == TransferSTRM {
		return ".strm"
	}
	return filepath.Ext(source)
}

// transferSTRMFile creates an exclusive, plain-text playback reference. It
// never moves or copies the source media, including across filesystem mounts.
func transferSTRMFile(src, dst string) error {
	if !isLocalSTRMFile(dst) {
		return fmt.Errorf("STRM destination must have a .strm extension")
	}
	target, err := organizeSTRMTarget(src)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644) // #nosec G304,G302 -- organizer-generated media reference must be readable by local players.
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(target + "\n")
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(dst)
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	return nil
}

func organizeSTRMTarget(src string) (string, error) {
	resolved, info, err := resolveAccessibleMappedPath(src)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || strings.ContainsAny(resolved, "\x00\r\n") {
		return "", fmt.Errorf("STRM source must be a regular media file: %s", src)
	}
	if _, ok := videoExtensions[strings.ToLower(filepath.Ext(resolved))]; !ok {
		return "", fmt.Errorf("unsupported STRM source: %s", src)
	}
	if !isLocalSTRMFile(resolved) {
		return filepath.Abs(resolved)
	}
	target, err := readLocalSTRMTarget(resolved)
	if err != nil {
		return "", err
	}
	if target == "" {
		return "", fmt.Errorf("STRM source has no supported playback target: %s", src)
	}
	// Preserve an existing STRM's actual target rather than chaining STRMs.
	// Resolve relative paths before changing the directory of the reference.
	if local, ok := localSTRMTargetPath(resolved, target); ok {
		return filepath.Abs(local)
	}
	return target, nil
}
