// Package service — storage configuration secret preservation helpers.
package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func shouldPreserveStorageSecretsOnSave(enabled *bool) bool {
	return enabled == nil || *enabled
}

func cloneStorageConfigMap(cfg map[string]any) map[string]any {
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}
	return out
}

func (s *StorageConfigService) mergeExistingStorageSecrets(ctx context.Context, typ string, cfg map[string]any) (map[string]any, error) {
	view, err := s.Get(ctx, typ)
	if err != nil || view == nil {
		return cfg, err
	}
	for _, key := range storagePreservedSecretKeys() {
		existing := strings.TrimSpace(strr(view.Config[key]))
		if existing == "" {
			continue
		}
		incoming, hasIncoming := cfg[key]
		if hasIncoming && !isBlankStorageSecret(incoming) {
			continue
		}
		if storageSecretReplacedByAlternative(typ, key, cfg, view.Config) {
			continue
		}
		cfg[key] = existing
	}
	return cfg, nil
}

func storagePreservedSecretKeys() []string {
	return []string{"password", "secret_key", "token", "cookie", "access_key"}
}

func isBlankStorageSecret(value any) bool {
	text := strings.TrimSpace(strr(value))
	return text == "" || text == "********"
}

func storageSecretReplacedByAlternative(typ, key string, cfg, existing map[string]any) bool {
	switch typ {
	case cloud.TypeOpenList:
		switch key {
		case "token":
			return strings.TrimSpace(strr(cfg["username"])) != "" && strings.TrimSpace(strr(cfg["password"])) != ""
		case "password":
			if strings.TrimSpace(strr(cfg["token"])) != "" {
				return true
			}
			return storagePlainFieldChanged("username", cfg, existing)
		}
	case "webdav", cloud.TypeCloudDrive2:
		if key == "password" {
			if strings.TrimSpace(strr(cfg["token"])) != "" {
				return true
			}
			return storagePlainFieldChanged("username", cfg, existing)
		}
	case "s3":
		if key == "secret_key" {
			return storagePlainFieldChanged("access_key", cfg, existing)
		}
	}
	return false
}

func storagePlainFieldChanged(key string, cfg, existing map[string]any) bool {
	incoming := strings.TrimSpace(strr(cfg[key]))
	if incoming == "" {
		return false
	}
	current := strings.TrimSpace(strr(existing[key]))
	return current != "" && incoming != current
}
