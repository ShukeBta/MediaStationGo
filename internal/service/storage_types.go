package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

const LegacyQuarkProvider = "quark"

func IsAdminStorageConfigurable(typ string) bool {
	switch strings.TrimSpace(typ) {
	case cloud.TypeOpenList, "alist", "webdav", cloud.TypeCloudDrive2, cloud.Type115:
		return true
	default:
		return false
	}
}

func IsAdminCloudConfigurable(typ string) bool {
	switch strings.TrimSpace(typ) {
	case cloud.Type115, cloud.TypeCloudDrive2, cloud.TypeOpenList:
		return true
	default:
		return false
	}
}

func IsDeprecatedNativeCloudProvider(typ string) bool {
	return strings.TrimSpace(typ) == LegacyQuarkProvider
}
