// Package model 定义第三方 API 配置数据模型。
package model

import (
	"time"
)

// APIConfig is the single schema for api_configs. Both configuration services
// share it so GORM cannot choose a partial definition during model ordering.
// Credentials are encrypted by the services and never serialized to JSON.
type APIConfig struct {
	Base
	ImageDirect     bool       `gorm:"default:false" json:"image_direct"`
	UseProxyPool    bool       `gorm:"default:false" json:"use_proxy_pool"`
	ProxyPoolType   string     `gorm:"size:16;default:normal" json:"-"`
	ResinProxyURL   string     `gorm:"size:512" json:"-"`
	ResinProxyToken string     `gorm:"type:text" json:"-"`
	ResinAccount    string     `gorm:"size:128" json:"-"`
	Provider        string     `gorm:"size:64;uniqueIndex;not null" json:"provider"`
	APIKey          string     `gorm:"type:text" json:"-"`
	BaseURL         string     `gorm:"size:512" json:"base_url,omitempty"`
	Extra           string     `gorm:"type:text" json:"extra,omitempty"`
	Enabled         bool       `gorm:"default:true" json:"enabled"`
	Description     string     `gorm:"size:255" json:"description,omitempty"`
	LastTestedAt    *time.Time `json:"last_tested_at,omitempty"`
	TestResult      string     `gorm:"size:32" json:"test_result,omitempty"`
}

func (APIConfig) TableName() string { return "api_configs" }

// ApiProvider 定义支持的 API 提供者列表。
type ApiProvider struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	HasAPIKey   bool   `json:"has_api_key"`
	HasBaseURL  bool   `json:"has_base_url"`
}

// PredefinedProviders 返回预定义的 API 提供者列表。
func PredefinedProviders() []ApiProvider {
	return []ApiProvider{
		{ID: "tmdb", Name: "TMDb", Description: "The Movie Database - 电影/剧集元数据", HasAPIKey: true, HasBaseURL: true},
		{ID: "douban", Name: "豆瓣", Description: "豆瓣电影/音乐/书籍数据", HasAPIKey: true, HasBaseURL: false},
		{ID: "bangumi", Name: "Bangumi", Description: "番剧/动漫数据库", HasAPIKey: true, HasBaseURL: false},
		{ID: "thetvdb", Name: "TheTVDB", Description: "TV Series Database", HasAPIKey: true, HasBaseURL: false},
		{ID: "fanart", Name: "Fanart.tv", Description: "影视海报/背景图", HasAPIKey: true, HasBaseURL: false},
		{ID: "openai", Name: "OpenAI", Description: "GPT 系列模型", HasAPIKey: true, HasBaseURL: true},
		{ID: "deepseek", Name: "DeepSeek", Description: "DeepSeek 大模型", HasAPIKey: true, HasBaseURL: true},
		{ID: "siliconflow", Name: "SiliconFlow", Description: "AI 模型聚合 API", HasAPIKey: true, HasBaseURL: true},
		{ID: "adult", Name: "Adult / 番号", Description: "JavDB/JavBus 成人内容元数据", HasAPIKey: false, HasBaseURL: true},
		{ID: "fd2ppv", Name: "FD2PPV", Description: "FD2PPV 会员账号与 FC2 元数据", HasAPIKey: true, HasBaseURL: true},
	}
}
