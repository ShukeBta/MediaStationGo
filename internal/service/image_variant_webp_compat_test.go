package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/gen2brain/webp"
	"go.uber.org/zap"
)

// Build a valid VP8X container around a lossless WebP. The leading odd-sized
// unknown chunk verifies RIFF alignment before the EXIF chunk is parsed.
func webpWithOrientation(t *testing.T, orientation byte, exifPrefix bool) []byte {
	return webpWithAlphaOrientation(t, orientation, exifPrefix, false)
}

func webpWithAlphaOrientation(t *testing.T, orientation byte, exifPrefix, transparent bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 30, B: 40, A: 255})
		}
	}
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if transparent {
		img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	}
	var encoded bytes.Buffer
	if err := webp.Encode(&encoded, img, webp.Options{Lossless: true}); err != nil {
		t.Fatal(err)
	}
	chunk := func(kind string, payload []byte) []byte {
		out := make([]byte, 8+len(payload)+(len(payload)&1))
		copy(out, kind)
		binary.LittleEndian.PutUint32(out[4:8], uint32(len(payload)))
		copy(out[8:], payload)
		return out
	}
	flags := []byte{8, 0, 0, 0, 2, 0, 0, 1, 0, 0}
	if transparent {
		flags[0] |= 0x10
	}
	body := append([]byte("WEBP"), chunk("VP8X", flags)...)
	body = append(body, encoded.Bytes()[12:]...)
	body = append(body, chunk("XTRA", []byte{1})...)
	exif := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, orientation, 0, 0, 0, 0, 0, 0, 0}
	if exifPrefix {
		exif = append([]byte("Exif\x00\x00"), exif...)
	}
	body = append(body, chunk("EXIF", exif)...)
	out := make([]byte, 8)
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(body)))
	return append(out, body...)
}

func TestImageVariantWebPTransparentLosslessWithExif(t *testing.T) {
	for _, prefixed := range []bool{false, true} {
		data := webpWithAlphaOrientation(t, 6, prefixed, true)
		out, _, err := buildImageVariant(data, "image/webp", imageVariantOptions{format: "png"})
		if err != nil {
			t.Fatal(err)
		}
		img, _, err := image.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 3 {
			t.Fatalf("orientation ignored: %v", img.Bounds())
		}
		pixel := color.NRGBAModel.Convert(img.At(1, 0)).(color.NRGBA)
		if pixel != (color.NRGBA{R: 255, A: 128}) {
			t.Fatalf("lossless RGB/alpha changed: %v", pixel)
		}
	}
}

func TestImageVariantWebPInvalidDimensionsSkipDecoderFallback(t *testing.T) {
	base := webpWithOrientation(t, 1, false)
	bitstream := bytes.Index(base, []byte("VP8L"))
	if bitstream < 0 {
		t.Fatal("missing VP8L fixture")
	}
	for name, mutate := range map[string]func([]byte) []byte{
		"canvas mismatch": func(data []byte) []byte {
			data[24], data[27] = 0, 0
			return data
		},
		"oversized inner VP8L": func(data []byte) []byte {
			data[24], data[27] = 0, 0
			binary.LittleEndian.PutUint32(data[bitstream+9:bitstream+13], 9999|(9999<<14))
			return data
		},
		"oversized inner VP8": func(data []byte) []byte {
			copy(data[bitstream:bitstream+4], "VP8 ")
			copy(data[bitstream+8:], []byte{0x10, 0, 0, 0x9d, 1, 0x2a, 0x10, 0x27, 0x10, 0x27})
			return data
		},
		"oversized canvas": func(data []byte) []byte {
			data[24], data[25], data[27], data[28] = 0xff, 0xff, 0xff, 0xff
			return data
		},
		"truncated RIFF": func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)))
			return data
		},
		"trailing data outside RIFF": func(data []byte) []byte {
			return append(data, []byte("VP8L\x05\x00\x00\x00\x2f\xff\xff\xff\x0f\x00")...)
		},
		"chunk crosses RIFF end": func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[4:8], uint32(bitstream+4))
			return data
		},
		"chunk length overflow": func(data []byte) []byte {
			binary.LittleEndian.PutUint32(data[bitstream+4:bitstream+8], 0xffffffff)
			return data
		},
		"duplicate bitstream": func(data []byte) []byte {
			length := int(binary.LittleEndian.Uint32(data[bitstream+4 : bitstream+8]))
			data = append(data, data[bitstream:bitstream+8+length+(length&1)]...)
			binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
			return data
		},
		"ALPH with VP8L": func(data []byte) []byte {
			alpha := []byte{'A', 'L', 'P', 'H', 2, 0, 0, 0, 0, 0}
			data = append(append(append([]byte{}, data[:bitstream]...), alpha...), data[bitstream:]...)
			binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
			return data
		},
	} {
		t.Run(name, func(t *testing.T) {
			data := mutate(bytes.Clone(base))
			proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
			var fallback atomic.Bool
			proxy.variantFallback = func(context.Context, []byte, imageVariantOptions) ([]byte, string, error) {
				fallback.Store(true)
				return nil, "", nil
			}
			req := httptest.NewRequest("GET", "/?format=png", nil)
			err := proxy.serveImageVariant(httptest.NewRecorder(), req, name, time.Unix(1, 0), int64(len(data)), data, "image/webp", "public", imageVariantOptions{format: "png"})
			if err == nil || fallback.Load() {
				t.Fatalf("invalid image escaped guard: err=%v fallback=%v", err, fallback.Load())
			}
		})
	}
}

func TestImageVariantWebPExifOrientationWithoutDecoderAutoRotate(t *testing.T) {
	for _, prefix := range []bool{false, true} {
		for orientation, point := range map[byte]image.Point{2: {2, 0}, 3: {2, 1}, 4: {0, 1}, 5: {0, 0}, 6: {1, 0}, 7: {1, 2}, 8: {0, 2}} {
			data := webpWithOrientation(t, orientation, prefix)
			decoded, ctype, err := buildImageVariant(data, "image/webp", imageVariantOptions{format: "png"})
			if err != nil {
				t.Fatal(err)
			}
			img, _, err := image.Decode(bytes.NewReader(decoded))
			if err != nil {
				t.Fatal(err)
			}
			width, height := 3, 2
			if orientation >= 5 {
				width, height = 2, 3
			}
			if ctype != "image/png" || img.Bounds().Dx() != width || img.Bounds().Dy() != height {
				t.Fatalf("orientation %d size=%v", orientation, img.Bounds())
			}
			r, g, b, a := img.At(point.X, point.Y).RGBA()
			if r != 65535 || g != 0 || b != 0 || a != 65535 {
				t.Fatalf("orientation %d corner=%d/%d/%d/%d", orientation, r, g, b, a)
			}
		}
	}
	// Malformed RIFF offsets must be bounded, and animation must pass through.
	malformed := webpWithOrientation(t, 6, false)
	binary.LittleEndian.PutUint32(malformed[16:20], 0xffffffff)
	if orientation, _ := variantMetadata(malformed, "webp"); orientation != 0 {
		t.Fatal("accepted out-of-range chunk")
	}
	animated := webpWithOrientation(t, 6, false)
	animated[20] |= 2
	if out, _, err := buildImageVariant(animated, "image/webp", imageVariantOptions{format: "png", maxWidth: 1}); err != nil || !bytes.Equal(out, animated) {
		t.Fatalf("animation flattened: %v", err)
	}
}

func TestImageVariantWebPLossyQualityRemainsEffective(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 96; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*31 + y*17), G: uint8(x*13 + y*29), B: uint8(x * y), A: 128})
		}
	}
	var source bytes.Buffer
	if err := webp.Encode(&source, img, webp.Options{Lossless: true}); err != nil {
		t.Fatal(err)
	}
	low, _, err := buildImageVariant(source.Bytes(), "image/webp", imageVariantOptions{format: "webp", quality: 35, hasQuality: true})
	if err != nil {
		t.Fatal(err)
	}
	high, _, err := buildImageVariant(source.Bytes(), "image/webp", imageVariantOptions{format: "webp", quality: 95, hasQuality: true})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(low, high) || len(low) >= len(high) {
		t.Fatalf("lossy quality ineffective sizes=%d/%d", len(low), len(high))
	}
	for _, data := range [][]byte{low, high} {
		out, _, err := buildImageVariant(data, "image/webp", imageVariantOptions{format: "png"})
		if err != nil {
			t.Fatal(err)
		}
		decoded, _, err := image.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, a := decoded.At(0, 0).RGBA()
		if a == 65535 || a == 0 {
			t.Fatal("alpha lost")
		}
	}
}
