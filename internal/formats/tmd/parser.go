package tmd

import (
	"fmt"

	"github.com/Xpl0itU/WiiUDownloader/internal/safebin"
)

type Content struct {
	ID    uint32
	Index [2]byte
	Type  uint16
	Size  uint64
	Hash  []byte
}

type Metadata struct {
	TitleID      uint64
	Version      byte
	TitleVersion uint16
	ContentCount uint16
	Contents     []Content
	Certificate1 []byte
	Certificate2 []byte
}

func Parse(data []byte) (*Metadata, error) {
	c := safebin.NewCursor(data)

	if err := c.Seek(versionOffset); err != nil {
		return nil, err
	}
	version, err := c.ReadU8()
	if err != nil {
		return nil, err
	}

	m := &Metadata{Version: version}
	switch version {
	case VersionWii:
		if err := parseHeader(c, m); err != nil {
			return nil, err
		}
		if err := parseContents(c, m, wiiContentStart, wiiContentStride, wiiHashSize); err != nil {
			return nil, err
		}
		if err := parseCertificates(c, m, wiiContentStart, wiiContentStride, int(m.ContentCount), wiiHashSize, true); err != nil {
			return nil, err
		}
	case VersionWiiU:
		if err := parseHeader(c, m); err != nil {
			return nil, err
		}
		if err := parseContents(c, m, wiiuContentStart, wiiuContentStride, wiiuHashSize); err != nil {
			return nil, err
		}
		if err := parseCertificates(c, m, wiiuContentStart, wiiuContentStride, int(m.ContentCount), wiiuHashSize, false); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown TMD version: %d", version)
	}
	return m, nil
}

func parseHeader(c *safebin.Cursor, m *Metadata) error {
	if err := c.Seek(titleIDOffset); err != nil {
		return err
	}
	titleID, err := c.ReadU64BE()
	if err != nil {
		return err
	}
	m.TitleID = titleID

	if err := c.Seek(titleVersionOffset); err != nil {
		return err
	}
	titleVersion, err := c.ReadU16BE()
	if err != nil {
		return err
	}
	contentCount, err := c.ReadU16BE()
	if err != nil {
		return err
	}
	m.TitleVersion = titleVersion
	m.ContentCount = contentCount
	return nil
}

func parseContents(c *safebin.Cursor, m *Metadata, start, stride, hashSize int) error {
	m.Contents = make([]Content, m.ContentCount)
	for i := 0; i < int(m.ContentCount); i++ {
		offset := start + (stride * i)
		if err := c.Seek(offset); err != nil {
			return err
		}

		id, err := c.ReadU32BE()
		if err != nil {
			return err
		}
		indexBytes, err := c.ReadBytes(2)
		if err != nil {
			return err
		}
		contentType, err := c.ReadU16BE()
		if err != nil {
			return err
		}
		size, err := c.ReadU64BE()
		if err != nil {
			return err
		}
		hash, err := c.ReadBytes(hashSize)
		if err != nil {
			return err
		}

		var idx [2]byte
		copy(idx[:], indexBytes)
		m.Contents[i] = Content{
			ID:    id,
			Index: idx,
			Type:  contentType,
			Size:  size,
			Hash:  append([]byte(nil), hash...),
		}
	}
	return nil
}

func parseCertificates(c *safebin.Cursor, m *Metadata, start, stride, count, hashSize int, required bool) error {
	if count == 0 {
		return nil
	}
	lastEnd := start + ((count - 1) * stride) + 16 + hashSize
	if lastEnd < 0 {
		return nil
	}
	if err := c.Seek(lastEnd); err != nil {
		if required {
			return err
		}
		return nil
	}
	if required || c.Remaining() >= 0x400 {
		b, err := c.ReadBytes(0x400)
		if err != nil {
			return err
		}
		m.Certificate1 = append([]byte(nil), b...)
	}
	if required || c.Remaining() >= 0x300 {
		b, err := c.ReadBytes(0x300)
		if err != nil {
			return err
		}
		m.Certificate2 = append([]byte(nil), b...)
	}
	return nil
}
