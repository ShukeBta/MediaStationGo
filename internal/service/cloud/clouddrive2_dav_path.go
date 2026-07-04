package cloud

import (
	"net/url"
	"strconv"
	"strings"
)

func (p *cloudDrive2Provider) urlFor(remotePath string) string {
	u := *p.base
	u.RawPath = ""
	basePath := strings.TrimRight(u.Path, "/")
	remote := strings.Trim(normalizeCloudDAVPath(remotePath), "/")
	switch {
	case basePath == "" || basePath == "/":
		if remote == "" {
			u.Path = "/"
		} else {
			u.Path = "/" + remote
		}
	case remote == "":
		u.Path = basePath
	default:
		u.Path = basePath + "/" + remote
	}
	return u.String()
}

func (p *cloudDrive2Provider) entryIDFromHref(href, basePath string) (string, error) {
	if href == "" {
		return "", nil
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	hrefPath := parsed.EscapedPath()
	if hrefPath == "" {
		hrefPath = href
	}
	if basePath != "" && basePath != "/" {
		hrefPath = strings.TrimPrefix(hrefPath, basePath)
	}
	if decoded, err := url.PathUnescape(hrefPath); err == nil {
		hrefPath = decoded
	}
	return normalizeCloudDAVPath(hrefPath), nil
}

const cloudDAVPropfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:">
  <d:prop>
    <d:displayname/>
    <d:getcontentlength/>
    <d:resourcetype/>
  </d:prop>
</d:propfind>`

type cloudDAVMultiStatus struct {
	Responses []cloudDAVResponse `xml:"response"`
}

type cloudDAVResponse struct {
	Href     string           `xml:"href"`
	PropStat cloudDAVPropStat `xml:"propstat"`
}

type cloudDAVPropStat struct {
	Prop cloudDAVProp `xml:"prop"`
}

type cloudDAVProp struct {
	DisplayName   string               `xml:"displayname"`
	ContentLength string               `xml:"getcontentlength"`
	ResourceType  cloudDAVResourceType `xml:"resourcetype"`
}

type cloudDAVResourceType struct {
	Collection *struct{} `xml:"collection"`
}

func parseDAVSize(raw string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return n
}
