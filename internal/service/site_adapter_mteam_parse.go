// Package service — M-Team search response parsing.
package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// parseMTeamJSON 解析 MTeam v3 JSON 响应。
//
// 响应结构（与旧版参考实现一致）：
//
//	{
//	  "code": "0",          // 字符串 "0" 表示成功
//	  "message": "SUCCESS",
//	  "data": {
//	    "total": "123",
//	    "data": [ ... ]    // 旧字段名 "lists" 已被替换为 "data"
//	  }
//	}
func parseMTeamJSON(data []byte, siteName, baseURL string) (*SiteSearchResult, error) {
	// 用 map 反序列化以兼容 code/total 既可能是字符串又可能是数字。
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	// code 兼容字符串与数字。
	codeStr := ""
	switch v := raw["code"].(type) {
	case string:
		codeStr = v
	case float64:
		codeStr = strconv.Itoa(int(v))
	}
	if codeStr != "" && codeStr != "0" && codeStr != "200" {
		msg, _ := raw["message"].(string)
		if msg == "" {
			msg = fmt.Sprintf("code=%s", codeStr)
		}
		return nil, fmt.Errorf("mteam: %s", msg)
	}

	dataField, _ := raw["data"].(map[string]interface{})
	if dataField == nil {
		return &SiteSearchResult{SiteName: siteName, Items: []TorrentItem{}}, nil
	}

	// total 兼容字符串与数字。
	total := 0
	switch v := dataField["total"].(type) {
	case string:
		total, _ = strconv.Atoi(v)
	case float64:
		total = int(v)
	}

	// data.data（v3）优先；兜底兼容旧的 data.lists。
	var rows []interface{}
	switch v := dataField["data"].(type) {
	case []interface{}:
		rows = v
	}
	if rows == nil {
		if v, ok := dataField["lists"].([]interface{}); ok {
			rows = v
		}
	}

	result := &SiteSearchResult{
		SiteName: siteName,
		Items:    []TorrentItem{},
		Total:    total,
	}

	for _, rawT := range rows {
		t, ok := rawT.(map[string]interface{})
		if !ok {
			continue
		}
		item := TorrentItem{}
		if v, ok := t["id"].(string); ok {
			item.ID = v
		} else if v, ok := t["id"].(float64); ok {
			item.ID = strconv.Itoa(int(v))
		}
		if v, ok := t["name"].(string); ok {
			item.Title = v
		}
		if v, ok := t["subtitle"].(string); ok {
			item.Subtitle = v
		}
		if v, ok := t["category"].(map[string]interface{}); ok {
			if name, ok := v["name"].(string); ok {
				item.Category = name
			}
		}
		if v, ok := t["size"].(float64); ok {
			item.Size = int64(v)
		} else if v, ok := t["size"].(string); ok {
			// v3 API 把 size 序列化成字符串。
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				item.Size = n
			}
		}
		if v, ok := t["status"].(map[string]interface{}); ok {
			if seeders, ok := v["seeders"].(float64); ok {
				item.Seeders = int(seeders)
			}
			if leechers, ok := v["leechers"].(float64); ok {
				item.Leechers = int(leechers)
			}
			if snatched, ok := v["completed"].(float64); ok {
				item.Snatched = int(snatched)
			}
		}
		if v, ok := t["free"].(bool); ok {
			item.Free = v
		}
		if v, ok := t["uploadTime"].(float64); ok {
			item.UploadTime = time.Unix(int64(v), 0)
		}

		item.DetailURL = baseURL + "/detail/" + item.ID
		// 标记 download_url 指向 genDlToken；真正的下载链接由 handler 层
		// 在用户点"下载"时通过 MTeamAdapter.GetDownloadURL 解析。
		// 这样前端 SiteSearchPage 才知道这一行有可用的下载入口。
		item.DownloadURL = baseURL + "/api/torrent/genDlToken?id=" + item.ID
		result.Items = append(result.Items, item)
	}

	return result, nil
}
