package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// GetAdapterForType 根据站点类型返回对应的适配器实例。
func GetAdapterForType(siteType string) SiteAdapter {
	switch strings.ToLower(siteType) {
	case "nexusphp":
		return NewNexusPHPAdapter()
	case "gazelle":
		return NewGazelleAdapter()
	case "unit3d":
		return NewUNIT3DAdapter()
	case "mteam":
		return NewMTeamAdapter()
	case "yemapt":
		return NewYemaPTAdapter()
	case "discuz":
		return NewDiscuzAdapter()
	case "custom_rss":
		return NewCustomRSSAdapter()
	default:
		return NewNexusPHPAdapter()
	}
}

// NewSiteAdapter 根据站点模型创建对应的适配器。
func NewSiteAdapter(site *model.Site) SiteAdapter {
	if site == nil {
		return nil
	}
	if isYemaPTURL(site.URL) {
		return NewYemaPTAdapter()
	}
	return GetAdapterForType(site.Type)
}
