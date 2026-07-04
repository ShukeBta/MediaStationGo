package service

// OrganizeResult reports what happened.
type OrganizeResult struct {
	Organized    int                     `json:"organized"`
	Skipped      int                     `json:"skipped"`
	Replaced     int                     `json:"replaced,omitempty"`
	Reclassified int                     `json:"reclassified,omitempty"`
	Errors       []string                `json:"errors,omitempty"`
	SourcePath   string                  `json:"source_path,omitempty"`
	DestPath     string                  `json:"dest_path,omitempty"`
	DryRun       bool                    `json:"dry_run,omitempty"`
	Items        []OrganizePreviewItem   `json:"items,omitempty"`
	Scans        []OrganizeScanSummary   `json:"scans,omitempty"`
	Scrapes      []OrganizeScrapeSummary `json:"scrapes,omitempty"`
}

type OrganizePreviewItem struct {
	Source    string `json:"source"`
	Target    string `json:"target,omitempty"`
	Action    string `json:"action"` // organize / skip / replace / reclassify / cleanup / error
	Reason    string `json:"reason,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Category  string `json:"category,omitempty"`
	Title     string `json:"title,omitempty"`
}

// OrganizeOptions carries per-request overrides for an organize operation.
// Empty values use system defaults.
//
// 整理是「从源目录整理到目的地目录」：SourcePath 指定待整理文件所在的源目录，
// DestPath 指定整理输出的目的地目录。两者相互独立，不再混用同一个目录。
type OrganizeOptions struct {
	// SourcePath 本次整理的源目录（待整理文件所在目录），覆盖 organize.source_dir
	// 设置与媒体库路径。仅整理位于该目录下的媒体；留空表示整个媒体库。
	SourcePath string
	// DestPath 本次整理的目的地根路径（整理输出到哪里），覆盖 organize.target_dir 设置。
	// 留空则使用设置中的默认目的地目录，再退回媒体库路径。
	DestPath string
	// TransferMode 本次整理的转移方式，覆盖 organize.transfer_mode 设置。
	TransferMode TransferMode
	// MediaType 手动整理时由 UI 指定的媒体类型。空值时按文件名/目录推断。
	MediaType string
	// MediaCategory 由订阅/下载任务或 UI 指定的分类。空值时按目录/NFO/规则推断。
	MediaCategory string
	// DryRun 仅生成整理预览，不实际移动/复制/硬链接文件。
	DryRun bool
	// AllowReplaceExisting 允许用本次来源替换目标库中已存在的同一媒体。
	// 默认 false：只去重不洗版，避免未开启洗版的订阅/手动整理留下或替换出多份版本。
	AllowReplaceExisting bool
}
