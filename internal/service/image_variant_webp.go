package service

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"

	"golang.org/x/image/vp8"
	"golang.org/x/image/vp8l"
)

type webpVariantSource struct {
	config   image.Config
	lossless []byte
	animated bool
	end      int
}

func validVariantPixels(width, height int) bool {
	return width > 0 && height > 0 && int64(width)*int64(height) <= 32_000_000
}

// Inspect both the canvas and actual bitstream before any decoder allocates
// pixels. DecodeConfig alone trusts VP8X even when VP8/VP8L says otherwise.
// Structural errors deliberately do not enable the FFmpeg decode fallback.
func inspectWebPVariant(data []byte) (source webpVariantSource, err error) {
	invalid := errors.New("invalid WebP image structure")
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return source, invalid
	}
	end := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if end < 12 || end != uint64(len(data)) || end&1 != 0 {
		return source, invalid
	}
	source.end = int(end)
	var canvas image.Config
	var extended, bitstream, alpha bool
	for pos := 12; pos < source.end; {
		if source.end-pos < 8 {
			return source, invalid
		}
		length := uint64(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		padded := length + length&1
		if padded > uint64(source.end-pos-8) {
			return source, invalid
		}
		payload := data[pos+8 : pos+8+int(length)]
		switch string(data[pos : pos+4]) {
		case "VP8X":
			if extended || pos != 12 || len(payload) != 10 {
				return source, invalid
			}
			extended = true
			canvas.Width = 1 + int(payload[4]) + int(payload[5])<<8 + int(payload[6])<<16
			canvas.Height = 1 + int(payload[7]) + int(payload[8])<<8 + int(payload[9])<<16
			if !validVariantPixels(canvas.Width, canvas.Height) {
				return source, errors.New("image pixel limit exceeded")
			}
			source.animated = payload[0]&2 != 0
		case "ANIM", "ANMF":
			if !extended || !source.animated {
				return source, invalid
			}
		case "ALPH":
			if alpha || bitstream || !extended {
				return source, invalid
			}
			alpha = true
		case "VP8 ", "VP8L":
			if bitstream {
				return source, invalid
			}
			bitstream = true
			if string(data[pos:pos+4]) == "VP8L" {
				if alpha {
					return source, invalid
				}
				source.config, err = vp8l.DecodeConfig(bytes.NewReader(payload))
				source.lossless = payload
			} else {
				decoder := vp8.NewDecoder()
				decoder.Init(bytes.NewReader(payload), len(payload))
				var header vp8.FrameHeader
				header, err = decoder.DecodeFrameHeader()
				source.config.Width, source.config.Height = header.Width, header.Height
			}
			if err != nil {
				return source, fmt.Errorf("invalid WebP bitstream header: %w", err)
			}
			if !validVariantPixels(source.config.Width, source.config.Height) {
				return source, errors.New("image pixel limit exceeded")
			}
			if extended && (canvas.Width != source.config.Width || canvas.Height != source.config.Height) {
				return source, errors.New("WebP canvas and bitstream dimensions differ")
			}
		}
		pos += 8 + int(padded)
	}
	if source.animated {
		source.config = canvas
	} else if !bitstream {
		return source, invalid
	}
	return source, nil
}
