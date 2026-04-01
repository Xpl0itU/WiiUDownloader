package wiiudownloader

import (
	"fmt"

	tmdfmt "github.com/Xpl0itU/WiiUDownloader/internal/formats/tmd"
)

const (
	TMD_VERSION_WII  = 0x00
	TMD_VERSION_WIIU = 0x01
)

type TMD struct {
	TitleID      uint64
	Version      byte
	TitleVersion uint16
	ContentCount uint16
	Contents     []Content
	Certificate1 []byte
	Certificate2 []byte
}

func (t *TMD) CalculateTotalSize() uint64 {
	var total uint64
	for _, content := range t.Contents {
		total += uint64(expectedContentDownloadSize(content))
		total += uint64(expectedH3DownloadSize(content))
	}
	return total
}


func ParseTMD(data []byte) (*TMD, error) {
	parsed, err := tmdfmt.Parse(data)
	if err != nil {
		return nil, err
	}

	out := &TMD{
		TitleID:      parsed.TitleID,
		Version:      parsed.Version,
		TitleVersion: parsed.TitleVersion,
		ContentCount: parsed.ContentCount,
		Contents:     make([]Content, len(parsed.Contents)),
		Certificate1: append([]byte(nil), parsed.Certificate1...),
		Certificate2: append([]byte(nil), parsed.Certificate2...),
	}
	for i, content := range parsed.Contents {
		out.Contents[i] = Content{
			ID:    content.ID,
			Index: append([]byte(nil), content.Index[:]...),
			Type:  content.Type,
			Size:  content.Size,
			Hash:  append([]byte(nil), content.Hash...),
		}
	}
	switch out.Version {
	case TMD_VERSION_WII, TMD_VERSION_WIIU:
		return out, nil
	default:
		return nil, fmt.Errorf("unknown TMD version: %d", out.Version)
	}
}
