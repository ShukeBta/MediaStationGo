package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const settingDownloadClientsManaged = "download_clients.managed"

// ReloadConfig rebuilds the qBittorrent client from the configured
// download clients (preferred) or the legacy Setting table (fallback).
//
// 配置来源优先级：
//
//  1. download_clients 表中 type=qbittorrent 且 is_default=true 且 enabled=true
//     的行（侧边栏「下载器」页面写入的数据）。
//  2. system Setting 表中的 qbittorrent.url / username / password
//     （旧版「系统设置」表单写入的数据；保留作向后兼容）。
//
// 这避免了两套配置各跑各的：之前操作员明明已经在「下载器」页面填好
// 默认 qb，但实际下载链路读的还是 Setting 表，导致一直连不上。
func (d *DownloadService) ReloadConfig(ctx context.Context) error {
	cfg := QBitConfig{}
	hasConfiguredClients := false
	managedByDownloadClients := false

	// Path 1: download_clients 表
	if d.repo.DownloadClient != nil {
		hasConfiguredClients, _ = d.repo.DownloadClient.HasAnyIncludingDeleted(ctx)
		if c, err := d.repo.DownloadClient.FindDefault(ctx); err == nil && c != nil && c.Type == "qbittorrent" {
			cfg.BaseURL = strings.TrimRight(c.Host, "/")
			cfg.Username = c.Username
			cfg.Password = c.Password
		} else if c, err := d.preferredEnabledQBitClient(ctx); err == nil && c != nil {
			cfg.BaseURL = strings.TrimRight(c.Host, "/")
			cfg.Username = c.Username
			cfg.Password = c.Password
			_ = d.repo.DownloadClient.SetDefault(ctx, c.ID)
			if d.log != nil {
				d.log.Warn("default downloader missing; selected first enabled qbittorrent client",
					zap.String("client_id", c.ID),
					zap.String("client", c.Name))
			}
		}
	}
	if d.repo.Setting != nil {
		managedRaw, _ := d.repo.Setting.Get(ctx, settingDownloadClientsManaged)
		managedByDownloadClients = strings.EqualFold(strings.TrimSpace(managedRaw), "true")
	}

	// Path 2: legacy Setting 表。
	// 仅在旧部署“从未使用过 download_clients 表”时回退。只要操作员曾经
	// 配置过下载器，删除/禁用全部下载器就表示应停止投递，不能再偷偷用
	// qbittorrent.* 旧设置继续往下载器添加任务。
	if cfg.BaseURL == "" && !hasConfiguredClients && !managedByDownloadClients {
		get := func(k string) string {
			v, _ := d.repo.Setting.Get(ctx, k)
			return v
		}
		cfg.BaseURL = get("qbittorrent.url")
		cfg.Username = get("qbittorrent.username")
		cfg.Password = get("qbittorrent.password")
	}

	d.qb.Configure(cfg)
	return nil
}

func (d *DownloadService) preferredEnabledQBitClient(ctx context.Context) (*model.DownloadClient, error) {
	if d == nil || d.repo == nil || d.repo.DownloadClient == nil {
		return nil, nil
	}
	rows, err := d.repo.DownloadClient.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	var selected *model.DownloadClient
	for i := range rows {
		if rows[i].Type != "qbittorrent" {
			continue
		}
		row := rows[i]
		selected = &row
		break
	}
	return selected, nil
}

func (d *DownloadService) defaultDownloaderNotConfiguredError(ctx context.Context) error {
	const prefix = "no default downloader configured"
	if d == nil || d.repo == nil || d.repo.DownloadClient == nil {
		return errors.New(prefix + ": 请在下载客户端中配置并启用 qBittorrent")
	}
	rows, err := d.repo.DownloadClient.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("%s: 读取下载客户端配置失败: %w", prefix, err)
	}
	if len(rows) == 0 {
		return errors.New(prefix + ": 请在下载客户端中启用 qBittorrent 并设为默认；当前没有已启用的下载器")
	}

	var enabled []string
	var hasQBit bool
	for _, row := range rows {
		label := strings.TrimSpace(row.Name)
		if label == "" {
			label = row.Type
		} else if row.Type != "" {
			label += "(" + row.Type + ")"
		}
		enabled = append(enabled, label)
		if strings.EqualFold(strings.TrimSpace(row.Type), "qbittorrent") {
			hasQBit = true
		}
	}
	if !hasQBit {
		return fmt.Errorf("%s: 订阅投递目前需要 qBittorrent；当前启用的下载器为 %s", prefix, strings.Join(enabled, ", "))
	}
	return errors.New(prefix + ": 请在下载客户端中选择一个启用的 qBittorrent 作为默认下载器")
}
