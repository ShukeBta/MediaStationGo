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

func TestDoubanDiscoverDetailLoadsRichMetadataAndUsesCache(t *testing.T) {
	var requests atomic.Int32
	provider := NewDoubanProvider(&config.Config{}, zap.NewNop())
	provider.client.Transport = doubanDetailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests.Add(1)
		body := ""
		switch req.URL.Path {
		case "/j/subject_abstract":
			body = `{"r":0,"subject":{"id":"10485526","title":"潜伏2 Insidious: Chapter 2 (2013)","rate":"8.2","subtype":"Movie","directors":["温子仁"],"actors":["帕特里克·威尔森","萝丝·拜恩"],"duration":"106分钟","region":"美国 / 加拿大","types":["惊悚","恐怖"],"release_year":"2013"}}`
		case "/subject/10485526/":
			body = doubanDetailPageFixture
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
		t.Fatalf("douban upstream requests = %d, want one abstract and one page request", requests.Load())
	}
	for _, item := range []ExternalMediaResult{first, second} {
		if item.DoubanID != "10485526" || item.ReleaseDate != "2013-09-13" || item.DurationMinutes != 106 {
			t.Fatalf("detail identity/date/duration = %#v", item)
		}
		if item.Overview != "完整剧情简介" || item.PosterURL != "https://img.example/poster.webp" || item.Rating != 8.2 {
			t.Fatalf("detail overview/poster/rating = %#v", item)
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

const doubanDetailPageFixture = `<!doctype html><html><head>
<script type="application/ld+json">{
  "name":"潜伏2 Insidious: Chapter 2",
  "image":"https://img.example/poster.webp",
  "director":[{"name":"温子仁"}],
  "author":[{"name":"雷·沃纳尔"},{"name":"温子仁"}],
  "actor":[{"name":"备用演员"}],
  "datePublished":"2013-09-13",
  "genre":["惊悚","恐怖"],
  "duration":"PT1H46M",
  "description":"摘要简介",
  "aggregateRating":{"ratingValue":"8.2"}
}</script></head><body>
<div id="info">
  <span class="pl">制片国家/地区:</span> 美国 / 加拿大<br>
  <span class="pl">语言:</span> 英语 / 法语<br>
  <span class="pl">又名:</span> 儿凶2 / 阴儿房第2章<br>
</div>
<span property="v:summary">完整剧情简介</span>
</body></html>`
