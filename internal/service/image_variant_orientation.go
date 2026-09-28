package service

import (
	"bytes"
	"encoding/binary"
	"image"
)

// variantMetadata 只读取方向与动画标记，不保留照片中的个人元数据。
func variantMetadata(data []byte, format string) (orientation int, animated bool) {
	if format == "jpeg" {
		for pos := 2; pos+4 <= len(data) && data[pos] == 0xff; {
			marker := data[pos+1]
			if marker == 0xda || marker == 0xd9 {
				break
			}
			if marker == 0xff {
				pos++
				continue
			}
			length := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
			if length < 2 || length > len(data)-pos-2 {
				break
			}
			payload := data[pos+4 : pos+2+length]
			if marker == 0xe1 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				return variantTIFFOrientation(payload[6:]), false
			}
			pos += length + 2
		}
	}
	if format == "png" {
		for pos := 8; pos+12 <= len(data); {
			length := uint64(binary.BigEndian.Uint32(data[pos : pos+4]))
			if length > uint64(len(data)-pos-12) {
				break
			}
			end := pos + 8 + int(length)
			switch string(data[pos+4 : pos+8]) {
			case "acTL":
				animated = true
			case "eXIf":
				orientation = variantTIFFOrientation(data[pos+8 : end])
			}
			pos = end + 4
		}
	}
	return orientation, animated
}

// variantTIFFOrientation 只解析 IFD0 的单个 SHORT 方向值，所有偏移先校验边界。
func variantTIFFOrientation(data []byte) int {
	if len(data) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(data[2:4]) != 42 {
		return 1
	}
	offset := uint64(order.Uint32(data[4:8]))
	if offset > uint64(len(data)-2) {
		return 1
	}
	count := int(order.Uint16(data[offset : offset+2]))
	pos := int(offset) + 2
	for i := 0; i < count && pos+12 <= len(data); i++ {
		entry := data[pos : pos+12]
		if order.Uint16(entry[:2]) == 0x112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
			value := int(order.Uint16(entry[8:10]))
			if value >= 1 && value <= 8 {
				return value
			}
			return 1
		}
		pos += 12
	}
	return 1
}

func orientVariantImage(src image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	wout, hout := w, h
	if orientation >= 5 {
		wout, hout = h, w
	}
	dst := image.NewNRGBA(image.Rect(0, 0, wout, hout))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch orientation {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
