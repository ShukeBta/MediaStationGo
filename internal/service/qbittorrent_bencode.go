package service

import (
	"crypto/sha1" // #nosec G505 -- BitTorrent v1 info-hash is SHA-1 by protocol.
	"encoding/hex"
)

func torrentInfoHash(data []byte) string {
	start, end, ok := torrentInfoBounds(data)
	if !ok {
		return ""
	}
	sum := sha1.Sum(data[start:end]) // #nosec G401 -- BitTorrent v1 info-hash is SHA-1 by protocol, not a security hash.
	return hex.EncodeToString(sum[:])
}

func torrentInfoBounds(data []byte) (int, int, bool) {
	if len(data) == 0 || data[0] != 'd' {
		return 0, 0, false
	}
	pos := 1
	for pos < len(data) && data[pos] != 'e' {
		keyStart, keyEnd, next, ok := parseBencodeString(data, pos)
		if !ok {
			return 0, 0, false
		}
		valueStart := next
		valueEnd, ok := bencodeValueEnd(data, valueStart)
		if !ok {
			return 0, 0, false
		}
		if string(data[keyStart:keyEnd]) == "info" {
			return valueStart, valueEnd, true
		}
		pos = valueEnd
	}
	return 0, 0, false
}

func parseBencodeString(data []byte, pos int) (int, int, int, bool) {
	if pos >= len(data) || data[pos] < '0' || data[pos] > '9' {
		return 0, 0, 0, false
	}
	length := 0
	for pos < len(data) && data[pos] >= '0' && data[pos] <= '9' {
		length = length*10 + int(data[pos]-'0')
		pos++
	}
	if pos >= len(data) || data[pos] != ':' {
		return 0, 0, 0, false
	}
	start := pos + 1
	end := start + length
	if end > len(data) {
		return 0, 0, 0, false
	}
	return start, end, end, true
}

func bencodeValueEnd(data []byte, pos int) (int, bool) {
	if pos >= len(data) {
		return 0, false
	}
	switch data[pos] {
	case 'i':
		end := pos + 1
		for end < len(data) && data[end] != 'e' {
			end++
		}
		if end >= len(data) {
			return 0, false
		}
		return end + 1, true
	case 'l', 'd':
		end := pos + 1
		for end < len(data) && data[end] != 'e' {
			next, ok := bencodeValueEnd(data, end)
			if !ok {
				return 0, false
			}
			end = next
		}
		if end >= len(data) {
			return 0, false
		}
		return end + 1, true
	default:
		_, _, next, ok := parseBencodeString(data, pos)
		return next, ok
	}
}
