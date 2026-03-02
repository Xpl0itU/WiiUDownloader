package u8

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	Magic        = 0x55AA382D
	MaxNodes     = 100000
	MaxFileSize  = 100 * 1024 * 1024
	MaxStackSize = 4096
)

type Node struct {
	Type       uint16
	NameOffset uint16
	DataOffset uint32
	Size       uint32
}

type Archive struct {
	Data             []byte
	RootNodeOffset   uint32
	HeaderSize       uint32
	DataOffset       uint32
	Nodes            []Node
	StringTable      []byte
	StringTableStart uint32
}

func Parse(data []byte) (*Archive, error) {
	if len(data) < 16 {
		return nil, errors.New("invalid U8 header: too short")
	}
	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != Magic {
		return nil, errors.New("invalid U8 magic")
	}
	rootNodeOffset := binary.BigEndian.Uint32(data[4:8])
	headerSize := binary.BigEndian.Uint32(data[8:12])
	dataOffset := binary.BigEndian.Uint32(data[12:16])
	if rootNodeOffset < 16 || int(rootNodeOffset)+12 > len(data) {
		return nil, fmt.Errorf("invalid U8 root node offset: %d", rootNodeOffset)
	}
	if dataOffset < rootNodeOffset || int(dataOffset) > len(data) {
		return nil, fmt.Errorf("invalid U8 data offset: %d", dataOffset)
	}

	root := parseNode(data[rootNodeOffset : rootNodeOffset+12])
	totalNodes := root.Size
	if totalNodes == 0 || totalNodes > MaxNodes {
		return nil, fmt.Errorf("invalid U8 node count: %d", totalNodes)
	}

	nodeTableSize := totalNodes * 12
	if nodeTableSize > uint32(len(data)) || rootNodeOffset+nodeTableSize > dataOffset || int(rootNodeOffset+nodeTableSize) > len(data) {
		return nil, fmt.Errorf("invalid U8 node table bounds")
	}
	nodes := make([]Node, totalNodes)
	for i := uint32(0); i < totalNodes; i++ {
		start := rootNodeOffset + i*12
		nodes[i] = parseNode(data[start : start+12])
	}

	stringTableStart := rootNodeOffset + nodeTableSize
	stringTableSize := dataOffset - stringTableStart
	if int(stringTableStart) > len(data) || int(stringTableStart+stringTableSize) > len(data) {
		return nil, fmt.Errorf("invalid U8 string table bounds")
	}

	return &Archive{
		Data:             data,
		RootNodeOffset:   rootNodeOffset,
		HeaderSize:       headerSize,
		DataOffset:       dataOffset,
		Nodes:            nodes,
		StringTable:      append([]byte(nil), data[stringTableStart:stringTableStart+stringTableSize]...),
		StringTableStart: stringTableStart,
	}, nil
}

func parseNode(data []byte) Node {
	return Node{
		Type:       binary.BigEndian.Uint16(data[0:2]),
		NameOffset: binary.BigEndian.Uint16(data[2:4]),
		DataOffset: binary.BigEndian.Uint32(data[4:8]),
		Size:       binary.BigEndian.Uint32(data[8:12]),
	}
}

func (a *Archive) Name(node Node) (string, error) {
	if int(node.NameOffset) >= len(a.StringTable) {
		return "", fmt.Errorf("invalid U8 name offset: %d", node.NameOffset)
	}
	start := int(node.NameOffset)
	end := start
	for end < len(a.StringTable) && a.StringTable[end] != 0 {
		end++
	}
	if end == len(a.StringTable) {
		return "", fmt.Errorf("unterminated U8 string at %d", node.NameOffset)
	}
	return string(a.StringTable[start:end]), nil
}

func Extract(data []byte, outputPath string) error {
	archive, err := Parse(data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return err
	}

	currentDir := outputPath
	dirStack := []string{outputPath}
	breakNodes := make([]uint32, 1, 128)
	breakNodes[0] = uint32(len(archive.Nodes))

	for i := uint32(1); i < uint32(len(archive.Nodes)); i++ {
		node := archive.Nodes[i]
		name, err := archive.Name(node)
		if err != nil {
			return err
		}
		cleanName, err := sanitizeName(name)
		if err != nil {
			return err
		}

		if node.Type == 0x0100 {
			nextDir, err := safeJoin(outputPath, currentDir, cleanName)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(nextDir, 0o755); err != nil {
				return err
			}
			currentDir = nextDir
			dirStack = append(dirStack, nextDir)
			breakNodes = append(breakNodes, node.Size)
			if len(dirStack) > MaxStackSize {
				return fmt.Errorf("U8 nesting too deep")
			}
		} else {
			if node.Size > MaxFileSize {
				continue
			}
			end := node.DataOffset + node.Size
			if end < node.DataOffset || int(end) > len(archive.Data) {
				return fmt.Errorf("U8 file node out of bounds")
			}
			targetPath, err := safeJoin(outputPath, currentDir, cleanName)
			if err != nil {
				return err
			}
			if err := os.WriteFile(targetPath, archive.Data[node.DataOffset:end], 0o644); err != nil {
				return err
			}
		}

		for len(breakNodes) > 1 && breakNodes[len(breakNodes)-1] == i+1 {
			breakNodes = breakNodes[:len(breakNodes)-1]
			dirStack = dirStack[:len(dirStack)-1]
			currentDir = dirStack[len(dirStack)-1]
		}
	}
	return nil
}

func sanitizeName(name string) (string, error) {
	if name == "" || name == "." {
		return "", fmt.Errorf("invalid empty U8 name")
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("invalid U8 name")
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("unsafe U8 path: %q", name)
	}
	return clean, nil
}

func safeJoin(base, currentDir, name string) (string, error) {
	target := filepath.Join(currentDir, name)
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if absTarget != absBase && !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe U8 extraction path: %q", name)
	}
	return absTarget, nil
}
