package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func organizeFatalResultError(res *OrganizeResult) error {
	if res == nil || len(res.Errors) == 0 {
		return nil
	}
	if res.Organized > 0 || res.Replaced > 0 || res.Reclassified > 0 {
		return nil
	}
	samples := organizeErrorSamples(res.Errors, 3)
	detail := strings.Join(samples, "; ")
	if detail == "" {
		detail = "unknown error"
	}
	return fmt.Errorf("organize failed: %d error(s), organized=0 replaced=0 skipped=%d: %s", len(res.Errors), res.Skipped, detail)
}

func organizeErrorSamples(errors []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, line := range errors {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (p *OrganizePipelineService) logOrganizeProblem(req OrganizePipelineRequest, res *OrganizeResult, err error) {
	if p == nil || p.log == nil || res == nil {
		return
	}
	fields := []zap.Field{
		zap.String("trigger", string(req.Trigger)),
		zap.String("scope", string(req.Scope)),
		zap.String("source", res.SourcePath),
		zap.String("dest", res.DestPath),
		zap.Int("organized", res.Organized),
		zap.Int("replaced", res.Replaced),
		zap.Int("reclassified", res.Reclassified),
		zap.Int("skipped", res.Skipped),
		zap.Int("errors", len(res.Errors)),
		zap.Strings("error_samples", organizeErrorSamples(res.Errors, 5)),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	p.log.Warn("organize pipeline completed with errors", fields...)
}

func (p *OrganizePipelineService) recordProblem(ctx context.Context, req OrganizePipelineRequest, res *OrganizeResult, err error) {
	if p == nil || p.repo == nil || p.repo.Log == nil {
		return
	}
	action := "organize.warning"
	if err != nil {
		action = "organize.failed"
	}
	target := strings.TrimSpace(req.SourcePath)
	detail := organizeAuditDetail(req, res, err)
	if res != nil {
		if strings.TrimSpace(res.SourcePath) != "" {
			target = res.SourcePath
		}
	} else if target == "" {
		target = firstNonEmpty(req.MediaID, req.LibraryID, req.DestPath)
	}
	row := &model.AccessLog{
		Action: action,
		Target: truncateForAccessLog(target, 255),
		Detail: truncateForAccessLog(detail, 4000),
	}
	if writeErr := p.repo.Log.Create(ctx, row); writeErr != nil && p.log != nil {
		p.log.Debug("organize audit log write failed", zap.Error(writeErr))
	}
}

func organizeAuditDetail(req OrganizePipelineRequest, res *OrganizeResult, err error) string {
	parts := []string{
		"trigger=" + string(req.Trigger),
		"scope=" + string(req.Scope),
	}
	if res != nil {
		parts = append(parts,
			fmt.Sprintf("source=%s", res.SourcePath),
			fmt.Sprintf("dest=%s", res.DestPath),
			fmt.Sprintf("organized=%d", res.Organized),
			fmt.Sprintf("replaced=%d", res.Replaced),
			fmt.Sprintf("reclassified=%d", res.Reclassified),
			fmt.Sprintf("skipped=%d", res.Skipped),
			fmt.Sprintf("errors=%d", len(res.Errors)),
		)
		if samples := organizeErrorSamples(res.Errors, 5); len(samples) > 0 {
			parts = append(parts, "error_samples="+strings.Join(samples, " | "))
		}
	}
	if err != nil {
		parts = append(parts, "error="+err.Error())
	}
	return strings.Join(parts, "\n")
}

func truncateForAccessLog(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
