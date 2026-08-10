package service

import "strings"

func normalizedTorrentProgress(progress float64) float64 {
	if progress < 0 {
		return 0
	}
	if progress > 1 {
		return 1
	}
	return progress
}

func canonicalTorrentState(state string, progress float64) string {
	state = strings.ToLower(strings.TrimSpace(state))
	complete := normalizedTorrentProgress(progress) >= 1
	switch state {
	case "completed", "complete", "seeding", "uploading", "stalledup", "pausedup", "queuedup", "forcedup":
		if complete || state == "completed" || state == "complete" || state == "seeding" {
			return "completed"
		}
		return "downloading"
	case "downloading", "forceddl", "metadl", "stalleddl", "active":
		if complete {
			return "completed"
		}
		return "downloading"
	case "queued", "queueddl", "download_pending", "seed_pending", "waiting":
		if complete {
			return "completed"
		}
		return "queued"
	case "paused", "pauseddl", "stoppeddl", "stopped":
		if complete {
			return "completed"
		}
		return "paused"
	case "checking", "checkingdl", "checkingup", "checkingresumedata", "check_pending", "moving":
		return "checking"
	case "error", "missingfiles":
		return "error"
	case "removed":
		return "removed"
	case "":
		if complete {
			return "completed"
		}
		return ""
	default:
		if complete {
			return "completed"
		}
		return state
	}
}

func downloaderPayloadPath(dir, name string) string {
	dir = strings.TrimSpace(dir)
	name = strings.TrimSpace(name)
	if name == "" {
		return dir
	}
	if dir == "" {
		return name
	}
	separator := "/"
	if strings.Contains(dir, `\`) && !strings.Contains(dir, "/") {
		separator = `\`
	}
	return strings.TrimRight(dir, `/\`) + separator + strings.TrimLeft(name, `/\`)
}

func downloaderPathBase(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), `/\`)
	if i := strings.LastIndexAny(value, `/\`); i >= 0 {
		return value[i+1:]
	}
	return value
}
