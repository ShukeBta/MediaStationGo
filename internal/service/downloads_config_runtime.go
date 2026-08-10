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

// ReloadConfig reloads managed adapters and preserves the legacy qBittorrent
// settings fallback for deployments that never used download_clients.
//
// 配置来源优先级：
//
//  1. download_clients 表中已启用的显式默认客户端；没有默认时持久化最早
//     创建的已启用客户端。
//  2. 从未使用多客户端下载器配置的旧部署，读取 system Setting 表中的
//     qbittorrent.url / username / password
//     （旧版「系统设置」表单写入的数据；保留作向后兼容）。
//
// 这避免了两套配置各跑各的：之前操作员明明已经在「下载器」页面填好
// 默认 qb，但实际下载链路读的还是 Setting 表，导致一直连不上。
func (d *DownloadService) ReloadConfig(ctx context.Context) error {
	if d.manager != nil {
		if err := d.manager.LoadAll(ctx); err != nil {
			return err
		}
		if d.manager.hasClients() {
			// Managed clients use their native adapters. Keep the legacy qB
			// client blank so a Transmission/aria2 default cannot be silently
			// overridden by an unrelated qB row.
			d.qb.Configure(QBitConfig{})
			return nil
		}
	}
	cfg := QBitConfig{}
	hasConfiguredClients := false
	managedByDownloadClients := false

	// Path 1: download_clients 表。此分支主要服务未注入 DownloadManager 的
	// 单元/兼容调用；生产容器已在上方通过原生适配器返回。
	if d.repo.DownloadClient != nil {
		hasConfiguredClients, _ = d.repo.DownloadClient.HasAnyIncludingDeleted(ctx)
		selected, _ := d.repo.DownloadClient.FindDefault(ctx)
		if selected == nil {
			selected, _ = d.preferredEnabledClient(ctx)
			if selected != nil {
				_ = d.repo.DownloadClient.SetDefault(ctx, selected.ID)
				selected.IsDefault = true
			}
		}
		if selected != nil {
			if d.log != nil {
				d.log.Debug("selected managed default downloader",
					zap.String("client_id", selected.ID),
					zap.String("client", selected.Name),
					zap.String("type", selected.Type))
			}
			if selected.Type == "qbittorrent" {
				cfg.BaseURL = strings.TrimRight(selected.Host, "/")
				cfg.Username = selected.Username
				cfg.Password = selected.Password
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

func (d *DownloadService) preferredEnabledClient(ctx context.Context) (*model.DownloadClient, error) {
	if d == nil || d.repo == nil || d.repo.DownloadClient == nil {
		return nil, nil
	}
	rows, err := d.repo.DownloadClient.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	selected := rows[0]
	return &selected, nil
}

func (d *DownloadService) defaultDownloaderNotConfiguredError(ctx context.Context) error {
	const prefix = "no default downloader configured"
	if d == nil || d.repo == nil || d.repo.DownloadClient == nil {
		return errors.New(prefix + ": 请在下载客户端中配置并启用下载器")
	}
	rows, err := d.repo.DownloadClient.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("%s: 读取下载客户端配置失败: %w", prefix, err)
	}
	if len(rows) == 0 {
		return errors.New(prefix + ": 请在下载客户端中启用下载器；当前没有已启用的下载器")
	}

	var enabled []string
	for _, row := range rows {
		label := strings.TrimSpace(row.Name)
		if label == "" {
			label = row.Type
		} else if row.Type != "" {
			label += "(" + row.Type + ")"
		}
		enabled = append(enabled, label)
	}
	return fmt.Errorf("%s: 请检查已启用下载器的连接和默认设置；当前启用的下载器为 %s", prefix, strings.Join(enabled, ", "))
}
