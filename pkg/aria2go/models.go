// Copyright (C) 2019 Vincent Chueng (coolingfall@gmail.com).

package aria2go

// Type definition for download information.
type DownloadInfo struct {
	Status         int
	TotalLength    int64
	BytesCompleted int64
	BytesUpload    int64
	DownloadSpeed  int
	UploadSpeed    int
	NumPieces      int
	Connections    int
	BitField       string
	InfoHash       string
	MetaInfo       MetaInfo
	Files          []File
	ErrorCode      int
	FollowedByGid  string
}

// Type definition for BitTorrent meta information.
type MetaInfo struct {
	Name         string
	AnnounceList []string
	Comment      string
	CreationUnix int64
	Mode         string
}

// Type definition for file in torrent.
type File struct {
	Index           int
	Name            string
	Length          int64
	CompletedLength int64
	Selected        bool
}

type Options map[string]string

// Type definition for download event, this will keep the same with aria2.
const (
	onStart = iota + 1
	onPause
	onStop
	onComplete
	onError
	onBTComplete
)
