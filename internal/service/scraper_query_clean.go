package service

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// yearPattern extracts a 4-digit year (1900-2099).
var yearPattern = regexp.MustCompile(`(?:^|[^\d])(19\d{2}|20\d{2})(?:[^\d]|$)`)

// noiseTokens are stripped before search.
var noiseTokens = []string{
	// 视频规格
	"1080p", "2160p", "4k", "720p", "480p", "uhd", "ds4k", "fhd",
	"bd", "bdrip", "brrip", "dvd", "dvdrip", "hdtv", "pdtv", "webdl",
	"hdrip", "bluray", "blu-ray", "webrip", "web-dl", "web",
	"x264", "x265", "h264", "h265", "h266", "hevc", "avc", "av1", "vvc", "10bit", "8bit", "hi10p", "hi10",
	"hdr", "hdr10", "sdr", "dts", "ddp", "ddp5", "dd5", "dd2", "eac3", "truehd",
	"dovi", "atmos", "aac", "ac3", "flac", "fps", "hlg", "dv",
	"remux", "extended", "uncut", "remastered", "repack", "proper", "internal",
	"limited", "imax", "directors-cut", "directors_cut",
	"hkfree", "yify", "rarbg", "ettv", "fgt", "tgx", "ctrlhd", "ntb", "flux", "qhstudio",

	// 流媒体平台 / 字幕组 / 国家版本（动漫常见）
	"netflix", "nf", "amzn", "hulu", "disney", "max", "hbo",
	"linetv", "ourtv", "iqiyi", "youku", "bilibili", "qiyi", "krj",
	"atvp", "appletv", "apple-tv", "tx", "txweb",
	"crunchyroll", "funimation", "anidb", "horriblesubs", "subsplease",
	"erai-raws", "judas", "asw", "smcat", "leopard-raws", "ohys-raws", "colortv",
	"mweb", "ubweb", "hhweb", "adweb", "chdweb", "kurosawa", "qhstudio",

	// 中文字幕标记
	"zm", "zw", "ch", "chs", "cht", "cn", "tc", "sc",
	"中字", "繁字", "简中", "繁中", "国语", "粤语", "日语",

	// 季数前缀残留 — ParseEpisode 已抽取过
	"season", "264", "265", "aac2", "aac5",
}

var noiseTokenSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(noiseTokens)+1)
	for _, token := range noiseTokens {
		set[token] = struct{}{}
	}
	set["dl"] = struct{}{}
	return set
}()

var releaseBoundaryTokenSet = map[string]struct{}{
	"1080p": {}, "2160p": {}, "4k": {}, "720p": {}, "480p": {}, "uhd": {}, "fhd": {},
	"bd": {}, "bdrip": {}, "brrip": {}, "dvd": {}, "dvdrip": {}, "hdtv": {}, "pdtv": {},
	"webdl": {}, "hdrip": {}, "bluray": {}, "webrip": {}, "web": {}, "remux": {},
	"x264": {}, "x265": {}, "h264": {}, "h265": {}, "h266": {}, "hevc": {}, "avc": {}, "av1": {}, "vvc": {},
}

var dynamicReleaseBoundaryTokenRE = regexp.MustCompile(`(?i)^(?:\d{3,4}p|\d{2,3}fps)$`)

// bracketedTag matches "[anything]", "(anything)" or "{anything}" segments.
var bracketedTag = regexp.MustCompile(`[\[\(\{][^\]\)\}]*[\]\)\}]`)
var multiWordNoise = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bweb[\s._-]*dl\b`),
	regexp.MustCompile(`(?i)\bblu[\s._-]*ray\b`),
	regexp.MustCompile(`(?i)\bdirectors[\s._-]*cut\b`),
	regexp.MustCompile(`(?i)\berai[\s._-]*raws\b`),
	regexp.MustCompile(`(?i)\bohys[\s._-]*raws\b`),
}

// CleanQuery converts a filename like "Inception.2010.1080p.BluRay.x264.mkv"
// into a TMDb-friendly title plus an optional year hint.
func CleanQuery(raw string) (title string, year int) {
	base := pathBaseSlash(raw)
	if base == "" {
		base = strings.TrimSpace(raw)
	}
	name := strings.TrimSuffix(base, filepath.Ext(base))
	lower := strings.ToLower(name)

	if m := yearPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			year = v
			lower = strings.ReplaceAll(lower, m[1], " ")
		}
	}

	lower = bracketedTag.ReplaceAllString(lower, " ")

	lower = patSEnE.ReplaceAllString(lower, " ")
	lower = patDanglingSE.ReplaceAllString(lower, " ")
	lower = patNxE.ReplaceAllString(lower, " ")
	lower = patEP.ReplaceAllString(lower, " ")
	lower = patCN.ReplaceAllString(lower, " ")
	// 去掉中文季/部标记（如「第二季」「第2部」），避免残留在标题里既污染
	// 搜索查询又导致整理后的目录名重复季信息。
	lower = patSeasonOnly.ReplaceAllString(lower, " ")
	lower = patCNSeason.ReplaceAllString(lower, " ")

	for _, pat := range multiWordNoise {
		lower = pat.ReplaceAllString(lower, " ")
	}
	for _, sep := range []string{".", "_", "-", "[", "]", "(", ")", "×"} {
		lower = strings.ReplaceAll(lower, sep, " ")
	}
	// 拆分后丢掉过短（≤1）且全为 ASCII 数字 / 字母的"碎片"，避免
	// 「2」「0」「v」之类残留干扰 TMDb 搜索。中文字符不算碎片。
	out := make([]string, 0, 8)
	seenReleaseBoundary := false
	for _, w := range strings.Fields(lower) {
		if dynamicReleaseBoundaryTokenRE.MatchString(w) {
			seenReleaseBoundary = true
			continue
		}
		if _, ok := noiseTokenSet[w]; ok {
			if _, boundary := releaseBoundaryTokenSet[w]; boundary {
				seenReleaseBoundary = true
			}
			continue
		}
		if seenReleaseBoundary && isASCIIWord(w) {
			continue
		}
		if len(w) <= 1 {
			r := []rune(w)
			if len(r) == 1 && r[0] < 128 {
				continue
			}
		}
		out = append(out, w)
	}
	title = strings.TrimSpace(strings.Join(out, " "))
	return title, year
}

func isASCIIWord(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= 128 {
			return false
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
