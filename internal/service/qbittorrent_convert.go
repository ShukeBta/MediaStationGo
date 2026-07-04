package service

// QBitTorrentToInfo 将旧的 QBitTorrent 转换为新的 TorrentInfo。
func QBitTorrentToInfo(q QBitTorrent) TorrentInfo {
	return TorrentInfo{
		Hash:      q.Hash,
		Name:      q.Name,
		Size:      q.Size,
		Progress:  float64(q.Progress),
		DLSpeed:   q.DLSpeed,
		UPSpeed:   q.UpSpeed,
		State:     q.State,
		SavePath:  q.SavePath,
		NumSeeds:  q.NumSeeds,
		NumLeechs: q.NumLeech,
	}
}

// TorrentInfoToQBit 将 TorrentInfo 转换回旧的 QBitTorrent 格式（兼容性）。
func TorrentInfoToQBit(t TorrentInfo) QBitTorrent {
	return QBitTorrent{
		Hash:     t.Hash,
		Name:     t.Name,
		State:    t.State,
		Progress: float32(t.Progress),
		DLSpeed:  t.DLSpeed,
		UpSpeed:  t.UPSpeed,
		NumSeeds: t.NumSeeds,
		NumLeech: t.NumLeechs,
		Size:     t.Size,
		SavePath: t.SavePath,
	}
}
