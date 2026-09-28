package service

import "strings"

func probeDefaultStreamIndexes(doc *ProbeDocument) (int, int) {
	audio, subtitle := -1, -1
	for _, stream := range doc.Streams {
		if stream.CodecType == "audio" && (audio < 0 || stream.Disposition.Default) {
			audio = stream.Index
		}
		if stream.CodecType == "subtitle" && (stream.Disposition.Default || (subtitle < 0 && stream.Disposition.Forced)) {
			subtitle = stream.Index
		}
	}
	return audio, subtitle
}

// Stored indexes are ffprobe's absolute indexes, including subtitle streams.
// External subtitles are appended after these by nextMediaStreamIndex.
func embyProbeStreams(doc *ProbeDocument) []map[string]any {
	tracks := projectProbeTracks(doc)
	streams := make([]map[string]any, 0, len(tracks))
	for _, track := range tracks {
		kind := strings.ToUpper(track.Type[:1]) + track.Type[1:]
		stream := map[string]any{
			"Index": track.Index, "Type": kind, "Codec": track.Codec,
			"Profile": track.Profile, "Level": track.Level, "TimeBase": track.TimeBase,
			"Language": track.Language, "DisplayLanguage": track.DisplayLanguage,
			"Title": track.Title, "DisplayTitle": track.DisplayTitle,
			"BitRate": track.BitRate, "IsDefault": track.IsDefault,
			"IsForced": track.IsForced, "IsExternal": false,
			"IsHearingImpaired": track.IsHearingImpaired, "IsVisualImpaired": track.IsVisualImpaired,
		}
		switch track.Type {
		case "video":
			stream["Width"], stream["Height"] = track.Width, track.Height
			stream["AspectRatio"], stream["PixelFormat"] = track.AspectRatio, track.PixelFormat
			stream["BitDepth"], stream["VideoRange"] = track.BitDepth, track.VideoRange
			stream["ColorRange"], stream["ColorSpace"] = track.ColorRange, track.ColorSpace
			stream["ColorTransfer"], stream["ColorPrimaries"] = track.ColorTransfer, track.ColorPrimaries
			stream["AverageFrameRate"], stream["RealFrameRate"] = track.AverageFrameRate, track.RealFrameRate
			stream["ExtendedVideoType"], stream["ExtendedVideoSubType"], stream["ExtendedVideoSubTypeDescription"] = probeExtendedVideoType(track.VideoRange)
		case "audio":
			stream["Channels"], stream["ChannelLayout"] = track.Channels, track.ChannelLayout
			stream["SampleRate"], stream["BitsPerSample"] = track.SampleRate, track.BitsPerSample
		case "subtitle":
			stream["IsTextSubtitleStream"] = track.IsTextSubtitle
			stream["SupportsExternalStream"] = false
		}
		streams = append(streams, stream)
	}
	return streams
}
