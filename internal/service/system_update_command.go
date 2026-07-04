package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func (s *SystemUpdateService) updateCommand(ctx context.Context, status SystemUpdateStatus) string {
	custom := s.rawUpdateCommand(ctx)
	if strings.TrimSpace(custom) != "" {
		return renderSystemUpdateCommand(custom, status)
	}
	return renderSystemUpdateCommand(defaultSystemUpdateCommand(), status)
}

func (s *SystemUpdateService) rawUpdateCommand(ctx context.Context) string {
	return s.setting(ctx, SystemUpdateCommandSettingKey, os.Getenv("MEDIASTATION_UPDATE_COMMAND"))
}

func defaultSystemUpdateCommand() string {
	return "cd {{compose_dir}} && {{compose_command}} pull && {{compose_command}} up -d && docker image prune -f && docker restart {{container}}"
}

func renderSystemUpdateCommand(template string, status SystemUpdateStatus) string {
	replacements := map[string]string{
		"{{image}}":            shellQuote(status.Image),
		"{{watchtower_image}}": shellQuote(firstNonEmpty(status.WatchtowerImage, DefaultSystemUpdateWatchtowerImage)),
		"{{compose_dir}}":      shellQuote(status.ComposeDir),
		"{{compose_file}}":     shellQuote(status.ComposeFile),
		"{{compose_command}}":  firstNonEmpty(status.ComposeCommand, "docker compose"),
		"{{container}}":        shellQuote(firstNonEmpty(status.ContainerName, status.ContainerID, "mediastation-go")),
		"{{container_id}}":     shellQuote(status.ContainerID),
		"{{container_name}}":   shellQuote(status.ContainerName),
	}
	out := template
	for marker, value := range replacements {
		out = strings.ReplaceAll(out, marker, value)
	}
	return strings.TrimSpace(out)
}

func runSystemUpdateCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func runSystemUpdateShell(ctx context.Context, command string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("empty update command")
	}
	if runtime.GOOS == "windows" {
		return runSystemUpdateCommand(ctx, "cmd", "/C", command)
	}
	return runSystemUpdateCommand(ctx, "/bin/sh", "-c", command)
}

func systemUpdateOutputDetails(output string) []string {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func redactSystemUpdateCommand(command string) string {
	command = strings.TrimSpace(command)
	if len(command) > 500 {
		return command[:500] + "..."
	}
	return command
}

func shellQuote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
