package service

import (
	"context"
	"strings"
)

func (p *OrganizePipelineService) startTask(ctx context.Context, req OrganizePipelineRequest, opts OrganizeOptions) *TaskHandle {
	if p == nil || p.tasks == nil {
		return nil
	}
	name := strings.TrimSpace(req.TaskName)
	if name == "" {
		name = p.defaultTaskName(req)
	}
	message := "正在整理/重命名/入库"
	if req.DryRun {
		message = "正在预览整理/重命名"
	}
	return p.tasks.Start(TaskKindOrganize, name, TaskUpdate{
		Stage:      "organize",
		SourcePath: firstNonEmpty(opts.SourcePath, p.defaultSourcePath(ctx, req)),
		DestPath:   firstNonEmpty(opts.DestPath, p.defaultDestPath(ctx, req)),
		Message:    message,
	})
}

func (p *OrganizePipelineService) finishTask(task *TaskHandle, err error, stage, message string, res *OrganizeResult) {
	if task == nil {
		return
	}
	task.Finish(err, TaskUpdate{
		Stage:   stage,
		Message: message,
		Metrics: OrganizeTaskMetrics(res),
		Details: OrganizeTaskDetails(res, 8),
	})
}

func (p *OrganizePipelineService) defaultTaskName(req OrganizePipelineRequest) string {
	switch req.Trigger {
	case OrganizeTriggerScheduled:
		return "自动整理重命名刮削入库"
	case OrganizeTriggerDownload:
		return "下载完成自动整理重命名刮削入库"
	default:
		if req.DryRun {
			return "预览整理重命名入库"
		}
		return "手动整理重命名刮削入库"
	}
}

func (p *OrganizePipelineService) failureMessage(req OrganizePipelineRequest) string {
	switch req.Trigger {
	case OrganizeTriggerScheduled:
		return "自动整理重命名入库失败"
	case OrganizeTriggerDownload:
		return "下载完成自动整理失败"
	default:
		return "手动整理重命名入库失败"
	}
}

func (p *OrganizePipelineService) completedMessage(req OrganizePipelineRequest) string {
	switch req.Trigger {
	case OrganizeTriggerScheduled:
		return "自动整理重命名刮削入库结束"
	case OrganizeTriggerDownload:
		return "下载完成自动整理入库结束"
	default:
		return "手动整理重命名刮削入库结束"
	}
}

func (p *OrganizePipelineService) defaultSourcePath(ctx context.Context, req OrganizePipelineRequest) string {
	if p == nil || p.organizer == nil {
		return ""
	}
	if req.Scope == OrganizeScopeDirectory {
		return p.organizer.defaultSourceRoot(ctx, "")
	}
	return ""
}

func (p *OrganizePipelineService) defaultDestPath(ctx context.Context, req OrganizePipelineRequest) string {
	if p == nil || p.organizer == nil {
		return ""
	}
	if req.Scope == OrganizeScopeDirectory {
		return p.organizer.defaultDestRoot(ctx, "")
	}
	return ""
}
