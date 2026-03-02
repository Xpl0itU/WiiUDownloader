package fst

import (
	"fmt"

	"github.com/Xpl0itU/WiiUDownloader/internal/safebin"
)

const (
	maxEntries = 1_000_000
)

type Entry struct {
	Type       byte
	NameOffset uint32
	Offset     uint32
	Length     uint32
	Flags      uint16
	ContentID  uint16
}

type Table struct {
	Factor      uint32
	EntryCount  uint32
	NamesOffset uint32
	Entries     []Entry
	data        []byte
}

func Parse(data []byte) (*Table, error) {
	c := safebin.NewCursor(data)
	if err := c.Seek(0x04); err != nil {
		return nil, err
	}
	factor, err := c.ReadU32BE()
	if err != nil {
		return nil, fmt.Errorf("failed to read factor: %w", err)
	}
	entryCount, err := c.ReadU32BE()
	if err != nil {
		return nil, fmt.Errorf("failed to read entry count: %w", err)
	}
	if factor == 0 {
		factor = 1
	}
	rootOffset := 0x20 + int(entryCount)*0x20
	if rootOffset < 0 || rootOffset > len(data)-16 {
		return nil, fmt.Errorf("invalid FST root offset")
	}

	if err := c.Seek(rootOffset + 8); err != nil {
		return nil, err
	}
	totalEntries, err := c.ReadU32BE()
	if err != nil {
		return nil, fmt.Errorf("failed to read entries from root: %w", err)
	}
	if totalEntries == 0 || totalEntries > maxEntries {
		return nil, fmt.Errorf("invalid FST entries count: %d", totalEntries)
	}

	namesOffset := uint32(0x20 + int(entryCount)*0x20 + int(totalEntries)*0x10)
	if int(namesOffset) > len(data) {
		return nil, fmt.Errorf("invalid FST names offset")
	}

	if err := c.Seek(rootOffset); err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, totalEntries)
	for i := uint32(0); i < totalEntries; i++ {
		typ, err := c.ReadU8()
		if err != nil {
			return nil, fmt.Errorf("failed to read type for entry %d: %w", i, err)
		}
		nameB, err := c.ReadBytes(3)
		if err != nil {
			return nil, fmt.Errorf("failed to read name offset for entry %d: %w", i, err)
		}
		nameOffset := uint32(nameB[2]) | uint32(nameB[1])<<8 | uint32(nameB[0])<<16
		offset, err := c.ReadU32BE()
		if err != nil {
			return nil, fmt.Errorf("failed to read content offset for entry %d: %w", i, err)
		}
		length, err := c.ReadU32BE()
		if err != nil {
			return nil, fmt.Errorf("failed to read length for entry %d: %w", i, err)
		}
		flags, err := c.ReadU16BE()
		if err != nil {
			return nil, fmt.Errorf("failed to read flags for entry %d: %w", i, err)
		}
		contentID, err := c.ReadU16BE()
		if err != nil {
			return nil, fmt.Errorf("failed to read content ID for entry %d: %w", i, err)
		}
		entries = append(entries, Entry{
			Type:       typ,
			NameOffset: nameOffset,
			Offset:     offset,
			Length:     length,
			Flags:      flags,
			ContentID:  contentID,
		})
	}

	return &Table{
		Factor:      factor,
		EntryCount:  entryCount,
		NamesOffset: namesOffset,
		Entries:     entries,
		data:        data,
	}, nil
}

func (t *Table) NameAt(nameOffset uint32) (string, error) {
	start := int(t.NamesOffset + nameOffset)
	if start < 0 || start >= len(t.data) {
		return "", fmt.Errorf("FST name offset out of bounds: %d", nameOffset)
	}

	end := start
	for end < len(t.data) && t.data[end] != 0 {
		end++
	}
	if end == len(t.data) {
		return "", fmt.Errorf("unterminated FST string at offset %d", nameOffset)
	}
	return string(t.data[start:end]), nil
}
