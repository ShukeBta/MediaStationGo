package service

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

type doubanDetailRoundTripFunc func(*http.Request) (*http.Response, error)

func (f doubanDetailRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoubanDiscoverDetailLoadsRexxarMetadataAndUsesCache(t *testing.T) {
	var requests atomic.Int32
	provider := NewDoubanProvider(&config.Config{}, zap.NewNop())
	provider.client.Transport = doubanDetailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		if got := req.Header.Get("Referer"); got != "https://m.douban.com/" {
			t.Fatalf("douban rexxar referer = %q", got)
		}
		body := ""
		switch req.URL.Path {
		case "/rexxar/api/v2/subject/10485526":
			body = doubanRexxarSubjectFixture
		case "/rexxar/api/v2/subject/10485526/credits":
			body = doubanRexxarCreditsFixture
		default:
			t.Fatalf("unexpected douban request: %s", req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})
	discover := NewDiscoverService(zap.NewNop(), nil).SetDouban(provider)

	first, err := discover.DoubanItemDetail(t.Context(), "movie", "10485526")
	if err != nil {
		t.Fatal(err)
	}
	second, err := discover.DoubanItemDetail(t.Context(), "movie", "10485526")
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("douban upstream requests = %d, want one subject and one credits request", requests.Load())
	}
	for _, item := range []ExternalMediaResult{first, second} {
		if item.DoubanID != "10485526" || item.ReleaseDate != "2013-09-13" || item.DurationMinutes != 106 {
			t.Fatalf("detail identity/date/duration = %#v", item)
		}
		if item.Title != "潜伏2" || item.OriginalName != "Insidious: Chapter 2" || item.Overview != "完整剧情简介" || item.PosterURL != "https://img.example/poster.webp" || item.Rating != 8.2 {
			t.Fatalf("detail title/overview/poster/rating = %#v", item)
		}
		if strings.Join(item.Directors, ",") != "温子仁" || strings.Join(item.Writers, ",") != "雷·沃纳尔,温子仁" {
			t.Fatalf("detail creators = %#v / %#v", item.Directors, item.Writers)
		}
		if strings.Join(item.Actors, ",") != "帕特里克·威尔森,萝丝·拜恩" {
			t.Fatalf("detail actors = %#v", item.Actors)
		}
		if strings.Join(item.Countries, ",") != "美国,加拿大" || strings.Join(item.Languages, ",") != "英语,法语" {
			t.Fatalf("detail countries/languages = %#v / %#v", item.Countries, item.Languages)
		}
		if strings.Join(item.Genres, ",") != "惊悚,恐怖" || strings.Join(item.Aliases, ",") != "儿凶2,阴儿房第2章" {
			t.Fatalf("detail genres/aliases = %#v / %#v", item.Genres, item.Aliases)
		}
	}
}

const doubanRexxarSubjectFixture = `{
  "id":"10485526",
  "title":"潜伏2",
  "original_title":"Insidious: Chapter 2",
  "intro":"完整剧情简介",
  "cover_url":"https://img.example/poster.webp",
  "year":"2013",
  "pubdate":["2013-09-13(美国)"],
  "durations":["106分钟"],
  "genres":["惊悚","恐怖"],
  "countries":["美国","加拿大"],
  "languages":["英语","法语"],
  "directors":[{"name":"温子仁"}],
  "actors":[{"name":"帕特里克·威尔森"},{"name":"萝丝·拜恩"}],
  "aka":["儿凶2","阴儿房第2章"],
  "is_tv":false,
  "type":"movie",
  "url":"https://movie.douban.com/subject/10485526/",
  "rating":{"value":8.2}
}`

const doubanRexxarCreditsFixture = `{
  "items":[
    {"name":"温子仁","category":"导演"},
    {"name":"雷·沃纳尔","category":"编剧"},
    {"name":"温子仁","category":"编剧"},
    {"name":"帕特里克·威尔森","category":"演员"}
  ],
  "total":4
}`
