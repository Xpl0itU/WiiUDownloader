package wiiudownloader

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	TMD_VERSION_WII  = 0x00
	TMD_VERSION_WIIU = 0x01
)

type TMD struct {
	Version      byte
	TitleVersion uint16
	ContentCount uint16
	Contents     []Content
}

func parseTMD(data []byte) (*TMD, error) {
	tmd := &TMD{}
	reader := bytes.NewReader(data)

	reader.Seek(0x180, io.SeekStart)
	if err := binary.Read(reader, binary.BigEndian, &tmd.Version); err != nil {
		return nil, err
	}

	switch tmd.Version {
	case TMD_VERSION_WII:
		reader.Seek(0x1DC, io.SeekStart)

		if err := binary.Read(reader, binary.BigEndian, &tmd.TitleVersion); err != nil {
			return nil, err
		}

		if err := binary.Read(reader, binary.BigEndian, &tmd.ContentCount); err != nil {
			return nil, err
		}

		tmd.Contents = make([]Content, tmd.ContentCount)

		for i := 0; i < int(tmd.ContentCount); i++ {
			offset := 0x1E4 + (0x24 * i)

			reader.Seek(int64(offset), io.SeekStart)
			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].ID); err != nil {
				return nil, err
			}

			reader.Seek(2, io.SeekCurrent)

			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].Type); err != nil {
				return nil, err
			}

			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].Size); err != nil {
				return nil, err
			}
		}
	case TMD_VERSION_WIIU:
		reader.Seek(0x1DC, io.SeekStart)

		if err := binary.Read(reader, binary.BigEndian, &tmd.TitleVersion); err != nil {
			return nil, err
		}

		if err := binary.Read(reader, binary.BigEndian, &tmd.ContentCount); err != nil {
			return nil, err
		}

		tmd.Contents = make([]Content, tmd.ContentCount)

		for i := 0; i < int(tmd.ContentCount); i++ {
			offset := 0xB04 + (0x30 * i)

			reader.Seek(int64(offset), io.SeekStart)
			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].ID); err != nil {
				return nil, err
			}

			reader.Seek(2, io.SeekCurrent)

			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].Type); err != nil {
				return nil, err
			}

			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].Size); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unknown TMD version: %d", tmd.Version)
	}
	return tmd, nil
}
