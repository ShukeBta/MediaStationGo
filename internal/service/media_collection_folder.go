package service

import (
	"regexp"
	"strings"
)

var (
	mediaCollectionEnglishRE = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:collection|anthology|trilogy|quadrilogy|tetralogy|saga|box[\s._-]*set)(?:[\s._-]|$)|(?:complete|all)[\s._-]*(?:\d+[\s._-]*)?(?:movies?|films?|collection|series|saga)(?:[\s._-]|$)|\d+[\s._-]*films?[\s._-]*collection`)
	mediaCollectionChineseRE = regexp.MustCompile(`(?:合集|全集|套装|全套|系列合集|电影系列|全\s*\d+\s*部)`)
)

func isMediaCollectionFolder(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && (mediaCollectionEnglishRE.MatchString(name) || mediaCollectionChineseRE.MatchString(name))
}

func mediaParentLooksLikeCollection(path string) bool {
	return isMediaCollectionFolder(pathBaseSlash(parentSlashPath(path)))
}
