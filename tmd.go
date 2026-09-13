package wiiudownloader

import (
	"fmt"

	tmdfmt "github.com/Xpl0itU/WiiUDownloader/internal/formats/tmd"
)

const (
	TMD_VERSION_WII  = tmdfmt.VersionWii
	TMD_VERSION_WIIU = tmdfmt.VersionWiiU
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
	return t.calculateTotalSize(nil)
}

// calculateTotalSize sums the download sizes of the given contents; a nil or
// empty selection means every content in the TMD.
func (t *TMD) calculateTotalSize(selected map[uint32]struct{}) uint64 {
	var total uint64
	for _, content := range t.Contents {
		if selected != nil {
			if _, ok := selected[content.ID]; !ok {
				continue
			}
		}
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
		Certificate1: parsed.Certificate1,
		Certificate2: parsed.Certificate2,
	}
	for i, content := range parsed.Contents {
		out.Contents[i] = Content{
			ID:    content.ID,
			Index: content.Index[:],
			Type:  content.Type,
			Size:  content.Size,
			Hash:  content.Hash,
		}
	}
	switch out.Version {
	case TMD_VERSION_WII, TMD_VERSION_WIIU:
		return out, nil
	default:
		return nil, fmt.Errorf("unknown TMD version: %d", out.Version)
	}
}
