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
	TitleID      uint64
	Version      byte
	TitleVersion uint16
	ContentCount uint16
	Contents     []Content
	Certificate1 []byte
	Certificate2 []byte
}

func ParseTMD(data []byte) (*TMD, error) {
	tmd := &TMD{}
	reader := bytes.NewReader(data)

	reader.Seek(0x180, io.SeekStart)
	if err := binary.Read(reader, binary.BigEndian, &tmd.Version); err != nil {
		return nil, err
	}

	switch tmd.Version {
	case TMD_VERSION_WII:
		reader.Seek(0x18C, io.SeekStart)
		if err := binary.Read(reader, binary.BigEndian, &tmd.TitleID); err != nil {
			return nil, err
		}

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

			reader.Seek(0x1E8+(0x24*int64(i)), io.SeekStart)
			tmd.Contents[i].Index = make([]byte, 2)
			if _, err := io.ReadFull(reader, tmd.Contents[i].Index); err != nil {
				return nil, err
			}

			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].Type); err != nil {
				return nil, err
			}

			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].Size); err != nil {
				return nil, err
			}

			tmd.Contents[i].Hash = make([]byte, 0x14)
			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[i].Hash); err != nil {
				return nil, err
			}
		}
		tmd.Certificate1 = make([]byte, 0x400)
		if _, err := io.ReadFull(reader, tmd.Certificate1); err != nil {
			return nil, err
		}
		tmd.Certificate2 = make([]byte, 0x300)
		if _, err := io.ReadFull(reader, tmd.Certificate2); err != nil {
			return nil, err
		}
	case TMD_VERSION_WIIU:
		reader.Seek(0x18C, io.SeekStart)
		if err := binary.Read(reader, binary.BigEndian, &tmd.TitleID); err != nil {
			return nil, err
		}
		reader.Seek(0x1DC, io.SeekStart)

		if err := binary.Read(reader, binary.BigEndian, &tmd.TitleVersion); err != nil {
			return nil, err
		}

		if err := binary.Read(reader, binary.BigEndian, &tmd.ContentCount); err != nil {
			return nil, err
		}

		tmd.Contents = make([]Content, tmd.ContentCount)

		for c := uint16(0); c < tmd.ContentCount; c++ {
			offset := 2820 + (48 * c)
			reader.Seek(int64(offset), io.SeekStart)
			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[c].ID); err != nil {
				return nil, err
			}

			reader.Seek(0xB08+(0x30*int64(c)), io.SeekStart)
			tmd.Contents[c].Index = make([]byte, 2)
			if _, err := io.ReadFull(reader, tmd.Contents[c].Index); err != nil {
				return nil, err
			}

			reader.Seek(0xB0A+(0x30*int64(c)), io.SeekStart)
			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[c].Type); err != nil {
				return nil, err
			}

			reader.Seek(0xB0C+(0x30*int64(c)), io.SeekStart)
			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[c].Size); err != nil {
				return nil, err
			}

			reader.Seek(0xB14+(0x30*int64(c)), io.SeekStart)
			tmd.Contents[c].Hash = make([]byte, 0x14)
			if err := binary.Read(reader, binary.BigEndian, &tmd.Contents[c].Hash); err != nil {
				return nil, err
			}
		}
		tmd.Certificate1 = make([]byte, 0x400)
		if _, err := io.ReadFull(reader, tmd.Certificate1); err != nil {
			return nil, err
		}
		tmd.Certificate2 = make([]byte, 0x300)
		if _, err := io.ReadFull(reader, tmd.Certificate2); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown TMD version: %d", tmd.Version)
	}
	return tmd, nil
}
