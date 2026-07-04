package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	ffmpegPixelSmashCVE          = "CVE-2026-8461"
	ffmpegPixelSmashFixedVersion = "8.1.2"
)

var ffmpegVersionRE = regexp.MustCompile(`(?i)\bff(?:mpeg|probe)\s+version\s+([0-9]+)\.([0-9]+)(?:\.([0-9]+))?`)

type FFmpegSecurityStatus struct {
	Status        string `json:"status"`
	CVE           string `json:"cve,omitempty"`
	FixedVersion  string `json:"fixed_version,omitempty"`
	Message       string `json:"message,omitempty"`
	ParsedVersion string `json:"parsed_version,omitempty"`
}

func EvaluateFFmpegSecurity(versionLine string) FFmpegSecurityStatus {
	major, minor, patch, parsed := parseFFmpegVersionLine(versionLine)
	if !parsed {
		return FFmpegSecurityStatus{
			Status:       "unknown",
			CVE:          ffmpegPixelSmashCVE,
			FixedVersion: ffmpegPixelSmashFixedVersion,
			Message:      "无法识别 FFmpeg 版本；请确认 ffmpeg/ffprobe 已更新到 8.1.2 或更高版本",
		}
	}
	status := FFmpegSecurityStatus{
		Status:        "ok",
		CVE:           ffmpegPixelSmashCVE,
		FixedVersion:  ffmpegPixelSmashFixedVersion,
		ParsedVersion: fmt.Sprintf("%d.%d.%d", major, minor, patch),
	}
	if major == 8 && ffmpegVersionLess(major, minor, patch, 8, 1, 2) {
		status.Status = "vulnerable"
		status.Message = "当前 FFmpeg 8.x 版本低于 8.1.2，可能受 PixelSmash 解码漏洞影响；请升级 ffmpeg/ffprobe"
		return status
	}
	if major < 8 {
		status.Status = "review"
		status.Message = "当前 FFmpeg 主版本低于 8；请按发行版安全公告确认是否已回补 PixelSmash 修复"
	}
	return status
}

func ffmpegVersionLess(major, minor, patch, fixedMajor, fixedMinor, fixedPatch int) bool {
	if major != fixedMajor {
		return major < fixedMajor
	}
	if minor != fixedMinor {
		return minor < fixedMinor
	}
	return patch < fixedPatch
}

func parseFFmpegVersionLine(versionLine string) (major, minor, patch int, ok bool) {
	match := ffmpegVersionRE.FindStringSubmatch(strings.TrimSpace(versionLine))
	if len(match) < 3 {
		return 0, 0, 0, false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, 0, false
	}
	minor, err = strconv.Atoi(match[2])
	if err != nil {
		return 0, 0, 0, false
	}
	if len(match) > 3 && match[3] != "" {
		patch, err = strconv.Atoi(match[3])
		if err != nil {
			return 0, 0, 0, false
		}
	}
	return major, minor, patch, true
}
