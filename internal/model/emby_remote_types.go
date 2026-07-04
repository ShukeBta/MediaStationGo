package model

import "time"

// EmbyRemoteSubtitleInfo 远程字幕信息。
type EmbyRemoteSubtitleInfo struct {
	ThreeLetterISOLanguageName string     `json:"ThreeLetterISOLanguageName"`
	Id                         string     `json:"Id"`
	ProviderName               string     `json:"ProviderName"`
	Name                       string     `json:"Name"`
	Format                     string     `json:"Format"`
	Author                     string     `json:"Author"`
	Comment                    string     `json:"Comment"`
	DateCreated                *time.Time `json:"DateCreated,omitempty"`
	CommunityRating            float64    `json:"CommunityRating,omitempty"`
	DownloadCount              int        `json:"DownloadCount"`
	IsHashMatch                bool       `json:"IsHashMatch,omitempty"`
	IsForced                   bool       `json:"IsForced,omitempty"`
	IsHearingImpaired          bool       `json:"IsHearingImpaired,omitempty"`
}

// EmbySubtitleSearchRequest 字幕搜索请求。
type EmbySubtitleSearchRequest struct {
	ItemId         string `json:"ItemId"`
	Language       string `json:"Language"`
	IsPerfectMatch bool   `json:"IsPerfectMatch,omitempty"`
}

// EmbyImageRemoteInfo 远程图片信息。
type EmbyImageRemoteInfo struct {
	Providers        []EmbyImageProviderInfo `json:"Providers"`
	TotalRecordCount int                     `json:"TotalRecordCount"`
}

// EmbyImageProviderInfo 图片提供者信息。
type EmbyImageProviderInfo struct {
	Name            string                `json:"Name"`
	RemoteImages    []EmbyRemoteImageInfo `json:"RemoteImages,omitempty"`
	SupportedImages []string              `json:"SupportedImages"`
}

// EmbyRemoteImageInfo 远程图片信息。
type EmbyRemoteImageInfo struct {
	Url             string  `json:"Url"`
	ThumbnailUrl    string  `json:"ThumbnailUrl,omitempty"`
	Height          int     `json:"Height"`
	Width           int     `json:"Width"`
	CommunityRating float64 `json:"CommunityRating,omitempty"`
	VoteCount       int     `json:"VoteCount,omitempty"`
	Language        string  `json:"Language,omitempty"`
	Type            string  `json:"Type"`
	RatingType      string  `json:"RatingType,omitempty"`
	ProviderName    string  `json:"ProviderName"`
}
