package service

import "os"

// Update playback and identity immediately, even when post-organize scanning
// is disabled. The STRM's file identity must not retain the source's inode.
func addOrganizedSTRMUpdates(updates map[string]any, path string) error {
	target, err := readLocalSTRMTarget(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	fileID, _ := fileIdentity(path)
	updates["container"] = "strm"
	updates["strm_url"] = target
	updates["file_id"] = fileID
	updates["size_bytes"] = info.Size()
	return nil
}
