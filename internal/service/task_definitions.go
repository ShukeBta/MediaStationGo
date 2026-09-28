package service

type TaskDefinition struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Trigger   string `json:"trigger"`
	Schedule  string `json:"schedule,omitempty"`
	Running   bool   `json:"running"`
	Manual    bool   `json:"manual"`
	LastError string `json:"last_error,omitempty"`
}

func schedulerTaskName(name string) string {
	names := map[string]string{
		"library_scan": "媒体库扫描", "cloud_sync": "云盘同步", "cloud_upload": "云盘元数据上传", "organize_source": "媒体整理",
		"transcode_cleanup": "转码缓存清理", "recycle_purge": "回收站清理", "tmdb_episode_recheck": "TMDb 季集资料复查", "tmdb_snapshot_backfill": "TMDb 快照回填",
		"fd2ppv_session_check": "FD2PPV 会话检查", "javdb_session_check": "JavDB 会话检查", "adult_discover_refresh": "发现资料刷新",
		"people_backfill_periodic": "人物资料回填", "people_translation_periodic": "人物翻译",
	}
	if label := names[name]; label != "" {
		return label
	}
	return name
}

func (c *Container) TaskDefinitions() []TaskDefinition {
	out := []TaskDefinition{}
	if c.Scheduler != nil {
		for _, job := range c.Scheduler.Status() {
			out = append(out, TaskDefinition{Key: job.Name, Name: schedulerTaskName(job.Name), Trigger: "定时 / 手动（沿用各任务设置）", Schedule: job.Interval, Running: job.Running, Manual: true, LastError: redactTaskText(job.LastErr)})
		}
	}
	for _, item := range []TaskDefinition{
		{Key: TaskKindScan, Name: "增量扫描", Trigger: "文件变更 / 媒体库操作"},
		{Key: TaskKindScrape, Name: "媒体刮削", Trigger: "入库 / 下方待刮削列表"},
		{Key: TaskKindProbe, Name: "媒体探测", Trigger: "入库 / 媒体详情操作"},
		{Key: TaskKindArtwork, Name: "媒体图片", Trigger: "入库 / 媒体库操作"},
		{Key: TaskKindSubtitle, Name: "字幕生成", Trigger: "播放器 / 下方字幕任务"},
		{Key: TaskKindPeople, Name: "人物资料与翻译", Trigger: "启用后每分钟处理待办 / 人物和媒体详情操作"},
		{Key: TaskKindDouban, Name: "豆瓣信息补齐", Trigger: "媒体详情手动操作"},
	} {
		if c.Tasks != nil {
			for _, task := range c.Tasks.Snapshot().Active {
				if task.Kind == item.Key {
					item.Running = true
				}
			}
		}
		out = append(out, item)
	}
	return out
}
