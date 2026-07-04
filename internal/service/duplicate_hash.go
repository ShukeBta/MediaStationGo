// Package service — duplicate sparse file hashing.
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

const sampleSize = 1 << 20 // 1 MiB per sample window

// SparseFileHash computes the head+mid+tail SHA-256 of a file, suffixed with
// the file size so two files that happen to collide on the sample window
// but differ in length are still distinguishable.
func SparseFileHash(path string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}
	f, err := os.Open(path) // #nosec G304 -- path is selected from configured media library files for duplicate detection.
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	size := st.Size()
	h := sha256.New()
	if size <= int64(sampleSize)*3 {
		if _, err := io.Copy(h, f); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s-%d", hex.EncodeToString(h.Sum(nil)), size), nil
	}
	buf := make([]byte, sampleSize)
	// head
	if _, err := io.ReadFull(f, buf); err != nil {
		return "", err
	}
	h.Write(buf)
	// middle
	if _, err := f.Seek(size/2-int64(sampleSize)/2, io.SeekStart); err != nil {
		return "", err
	}
	if _, err := io.ReadFull(f, buf); err != nil {
		return "", err
	}
	h.Write(buf)
	// tail
	if _, err := f.Seek(size-int64(sampleSize), io.SeekStart); err != nil {
		return "", err
	}
	if _, err := io.ReadFull(f, buf); err != nil {
		return "", err
	}
	h.Write(buf)
	return fmt.Sprintf("%s-%d", hex.EncodeToString(h.Sum(nil)), size), nil
}
