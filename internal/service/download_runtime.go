package service

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const legacyQBitDownloadClientID = "legacy-qbittorrent"

type downloadTarget struct {
	clientID string
	typ      string
	adapter  DownloadAdapter
	legacyQB bool
}

func (d *DownloadService) listLiveTorrents(ctx context.Context, filter string) ([]QBitTorrent, error) {
	if d != nil && d.manager != nil && d.manager.hasClients() {
		var live []QBitTorrent
		var listErrs []error
		for _, target := range d.manager.targets() {
			items, err := target.adapter.List(ctx, filter)
			if err != nil {
				listErrs = append(listErrs, fmt.Errorf("%s (%s): %w", target.client.Name, target.client.Type, err))
			}
			for _, item := range items {
				torrent := TorrentInfoToQBit(item)
				torrent.ClientID = target.client.ID
				torrent.Source = target.client.Type
				live = append(live, torrent)
			}
		}
		return live, errors.Join(listErrs...)
	}
	if d != nil && d.qb != nil && d.qb.IsConfigured() {
		live, err := d.qb.List(ctx, filter)
		for i := range live {
			live[i].ClientID = legacyQBitDownloadClientID
			live[i].Source = "qbittorrent"
			live[i].Progress = float32(normalizedTorrentProgress(float64(live[i].Progress)))
			live[i].State = canonicalTorrentState(live[i].State, float64(live[i].Progress))
		}
		return live, err
	}
	return nil, errors.New("no download client available")
}

func torrentURLInfoHash(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(parsed.Scheme, "magnet") {
		return ""
	}
	for _, xt := range parsed.Query()["xt"] {
		const prefix = "urn:btih:"
		if strings.HasPrefix(strings.ToLower(xt), prefix) {
			return normalizeTorrentInfoHash(xt[len(prefix):])
		}
	}
	return ""
}

func normalizeTorrentInfoHash(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 40 {
		if _, err := hex.DecodeString(value); err == nil {
			return strings.ToLower(value)
		}
	}
	if len(value) == 32 {
		decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(value))
		if err == nil && len(decoded) == 20 {
			return hex.EncodeToString(decoded)
		}
	}
	return strings.ToLower(value)
}

func (d *DownloadService) defaultDownloadTarget(ctx context.Context) (downloadTarget, error) {
	if d != nil && d.manager != nil {
		client, adapter, err := d.manager.GetDefault(ctx)
		if err == nil && client != nil && adapter != nil {
			return downloadTarget{clientID: client.ID, typ: client.Type, adapter: adapter}, nil
		}
	}
	if d != nil && d.qb != nil && d.qb.IsConfigured() {
		return downloadTarget{clientID: legacyQBitDownloadClientID, typ: "qbittorrent", legacyQB: true}, nil
	}
	return downloadTarget{}, errors.New("no default downloader configured: 请在下载客户端中配置并启用默认下载器")
}

func (d *DownloadService) downloadTargetByID(ctx context.Context, clientID string) (downloadTarget, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return d.defaultDownloadTarget(ctx)
	}
	if clientID == legacyQBitDownloadClientID {
		if d != nil && d.qb != nil && d.qb.IsConfigured() {
			return downloadTarget{clientID: legacyQBitDownloadClientID, typ: "qbittorrent", legacyQB: true}, nil
		}
		return downloadTarget{}, errors.New("legacy qbittorrent is not configured")
	}
	if d != nil && d.manager != nil {
		target, err := d.manager.getTarget(clientID)
		if err == nil {
			return downloadTarget{clientID: target.client.ID, typ: target.client.Type, adapter: target.adapter}, nil
		}
	}
	return downloadTarget{}, errors.New("download client not found or disabled")
}

func (d *DownloadService) resolveOperationClientID(ctx context.Context, externalID, requestedClientID string) (string, string, error) {
	requestedClientID = strings.TrimSpace(requestedClientID)
	live, listErr := d.listLiveTorrents(ctx, "")
	if clientID, name, err, resolved := resolveLiveOperationClient(live, externalID, requestedClientID); resolved {
		return clientID, name, err
	}
	if requestedClientID != "" {
		return requestedClientID, "", nil
	}
	if clientID, err := d.persistedOperationClientID(ctx, externalID); clientID != "" || err != nil {
		return clientID, "", err
	}
	if clientID, err := d.singleAvailableOperationClientID(); clientID != "" || err != nil {
		return clientID, "", err
	}
	if d != nil && d.qb != nil && d.qb.IsConfigured() {
		return legacyQBitDownloadClientID, "", nil
	}
	if listErr != nil {
		return "", "", listErr
	}
	return "", "", errors.New("download client not found")
}

func resolveLiveOperationClient(live []QBitTorrent, externalID, requestedClientID string) (string, string, error, bool) {
	var matched []QBitTorrent
	for _, torrent := range live {
		if requestedClientID != "" && torrent.ClientID != requestedClientID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(torrent.Hash), strings.TrimSpace(externalID)) {
			matched = append(matched, torrent)
		}
	}
	if len(matched) == 1 {
		return matched[0].ClientID, matched[0].Name, nil, true
	}
	if len(matched) > 1 && requestedClientID == "" {
		return "", "", errors.New("multiple download clients contain this task; client_id is required"), true
	}
	return "", "", nil, false
}

func (d *DownloadService) persistedOperationClientID(ctx context.Context, externalID string) (string, error) {
	if d != nil && d.repo != nil && d.repo.Download != nil {
		rows, err := d.repo.Download.List(ctx)
		if err != nil {
			return "", err
		}
		var clientID string
		for _, row := range rows {
			if !strings.EqualFold(strings.TrimSpace(row.ExternalID), strings.TrimSpace(externalID)) || strings.TrimSpace(row.DownloadClientID) == "" {
				continue
			}
			if clientID != "" && clientID != row.DownloadClientID {
				return "", errors.New("multiple download clients contain this task; client_id is required")
			}
			clientID = row.DownloadClientID
		}
		return clientID, nil
	}
	return "", nil
}

func (d *DownloadService) singleAvailableOperationClientID() (string, error) {
	if d != nil && d.manager != nil {
		targets := d.manager.targets()
		if len(targets) == 1 {
			return targets[0].client.ID, nil
		}
		if len(targets) > 1 {
			return "", errors.New("client_id is required when multiple download clients are enabled")
		}
	}
	return "", nil
}
