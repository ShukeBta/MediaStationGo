package service

import (
	"encoding/json"
	"time"
)

// parseAria2Items 解析 Aria2 返回的任务列表。
func (a *Aria2Adapter) parseAria2Items(raw json.RawMessage) []TorrentInfo {
	var items []map[string]interface{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil
	}

	result := make([]TorrentInfo, 0, len(items))
	for _, item := range items {
		info := a.parseSingleItem(item)
		if info != nil {
			result = append(result, *info)
		}
	}
	return result
}

// parseSingleItem 解析单个 Aria2 任务项。
func (a *Aria2Adapter) parseSingleItem(item map[string]interface{}) *TorrentInfo {
	gid := strVal(item["gid"])
	totalLength := toInt64(item["totalLength"])
	completedLength := toInt64(item["completedLength"])
	dlSpeed := toInt64(item["downloadSpeed"])
	upSpeed := toInt64(item["uploadSpeed"])
	status := strVal(item["status"])
	dir := strVal(item["dir"])
	numSeeders := int(toInt64(item["numSeeders"]))
	connections := int(toInt64(item["connections"]))

	var name string
	var hash string

	// 尝试从 bittorrent info 获取名称和 hash
	if bt, ok := item["bittorrent"].(map[string]interface{}); ok {
		if info, ok := bt["info"].(map[string]interface{}); ok {
			name = strVal(info["name"])
		}
		hash = strVal(bt["infoHash"])
	}

	// 如果没有 bittorrent 信息，使用 GID 作为 hash
	if hash == "" {
		hash = gid
	}
	if name == "" {
		// 尝试从 files 获取文件名
		if files, ok := item["files"].([]interface{}); ok && len(files) > 0 {
			if f, ok := files[0].(map[string]interface{}); ok {
				paths, ok := f["path"].([]interface{})
				if ok && len(paths) > 0 {
					name = strVal(paths[len(paths)-1])
				}
				if name == "" {
					name = strVal(f["uris"])
				}
			}
		}
	}
	if name == "" {
		name = gid
	}

	var progress float64
	if totalLength > 0 {
		progress = float64(completedLength) / float64(totalLength) * 100
	}

	// Aria2 状态映射
	state := aria2StatusStr(status)

	return &TorrentInfo{
		Hash:      hash,
		Name:      name,
		Size:      totalLength,
		Progress:  progress,
		DLSpeed:   dlSpeed,
		UPSpeed:   upSpeed,
		State:     state,
		SavePath:  dir,
		NumSeeds:  numSeeders,
		NumLeechs: aria2MaxInt(connections-numSeeders, 0),
		AddedOn:   time.Now(),
	}
}

// aria2StatusStr 将 Aria2 状态转为可读字符串。
func aria2StatusStr(status string) string {
	switch status {
	case "active":
		return "downloading"
	case "waiting":
		return "queued"
	case "paused":
		return "paused"
	case "error":
		return "error"
	case "complete":
		return "seeding"
	case "removed":
		return "removed"
	default:
		return status
	}
}

func aria2MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
