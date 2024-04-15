package wiiudownloader

import (
	"bytes"
	"encoding/binary"
	"io"
)

type TMD struct {
	TitleVersion uint16
	ContentCount uint16
	Contents     []Content
}

func parseTMD(data []byte) (*TMD, error) {
	tmd := &TMD{}
	reader := bytes.NewReader(data)

	reader.Seek(476, io.SeekStart)

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
	return tmd, nil
}
