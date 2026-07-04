package service

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
)

// CheckFFmpegStatus 检查 ffmpeg/ffprobe 状态 (供 API 使用)
func CheckFFmpegStatus(ffprobePath, ffmpegPath string) map[string]interface{} {
	status := map[string]interface{}{
		"ffprobe_installed": false,
		"ffmpeg_installed":  false,
		"auto_installable":  runtime.GOOS == "windows",
	}

	if ffprobePath != "" {
		if _, err := os.Stat(ffprobePath); err == nil {
			status["ffprobe_installed"] = true
			status["ffprobe_path"] = ffprobePath

			// 获取版本
			cmd := exec.Command(ffprobePath, "-version")
			out, err := cmd.Output()
			if err == nil {
				// 提取版本信息（第一行）
				lines := bytes.Split(out, []byte("\n"))
				if len(lines) > 0 {
					version := string(bytes.TrimSpace(lines[0]))
					status["ffprobe_version"] = version
					status["ffprobe_security"] = EvaluateFFmpegSecurity(version)
				}
			}
		}
	}

	if ffmpegPath != "" {
		if _, err := os.Stat(ffmpegPath); err == nil {
			status["ffmpeg_installed"] = true
			status["ffmpeg_path"] = ffmpegPath

			cmd := exec.Command(ffmpegPath, "-version")
			out, err := cmd.Output()
			if err == nil {
				lines := bytes.Split(out, []byte("\n"))
				if len(lines) > 0 {
					version := string(bytes.TrimSpace(lines[0]))
					status["ffmpeg_version"] = version
					status["ffmpeg_security"] = EvaluateFFmpegSecurity(version)
				}
			}
		}
	}

	return status
}
