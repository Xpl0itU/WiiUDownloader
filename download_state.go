package wiiudownloader

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultDownloadSegmentSize = 1 << 20
	downloadPartExtension      = ".part"
	downloadStateExtension     = ".resume.json"
	downloadStateDirPerm       = 0o755
	downloadFilePerm           = 0o644
)

type downloadOptions struct {
	ExpectedSize int64
	DoRetries    bool
	AllowResume  bool
	SegmentSize  int64
	UserAgent    string
	Validate     func(path string) error
}

type downloadSegment struct {
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type downloadState struct {
	URL            string            `json:"url"`
	ExpectedSize   int64             `json:"expected_size"`
	SegmentSize    int64             `json:"segment_size"`
	VerifiedOffset int64             `json:"verified_offset"`
	LastModified   string            `json:"last_modified,omitempty"`
	ETag           string            `json:"etag,omitempty"`
	Segments       []downloadSegment `json:"segments"`
	PartialSegment *downloadSegment  `json:"partial_segment,omitempty"`
}

type resumeStateWriter struct {
	file        *os.File
	state       *downloadState
	statePath   string
	segmentSize int64
	pending     []byte
	pendingAt   int64
}

func partPathFor(dstPath string) string {
	return dstPath + downloadPartExtension
}

func statePathFor(dstPath string) string {
	return dstPath + downloadStateExtension
}

func hashSegmentHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadDownloadState(path string) (downloadState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return downloadState{}, err
	}
	var state downloadState
	if err := json.Unmarshal(data, &state); err != nil {
		return downloadState{}, err
	}
	if state.SegmentSize <= 0 {
		state.SegmentSize = defaultDownloadSegmentSize
	}
	return state, nil
}

func saveDownloadState(path string, state downloadState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, downloadStateDirPerm); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, downloadFilePerm); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func normalizeSegmentSize(segmentSize int64) int64 {
	if segmentSize > 0 {
		return segmentSize
	}
	return defaultDownloadSegmentSize
}

func resetDownloadState(downloadURL string, expectedSize int64, segmentSize int64) downloadState {
	return downloadState{
		URL:          downloadURL,
		ExpectedSize: expectedSize,
		SegmentSize:  normalizeSegmentSize(segmentSize),
		Segments:     make([]downloadSegment, 0),
	}
}

func cleanupPartialDownload(dstPath string) error {
	if err := os.Remove(partPathFor(dstPath)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(statePathFor(dstPath)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func prepareDownloadState(dstPath, downloadURL string, expectedSize int64, allowResume bool, segmentSize int64) (*downloadState, int64, error) {
	if !allowResume {
		if err := cleanupPartialDownload(dstPath); err != nil {
			return nil, 0, err
		}
		return nil, 0, nil
	}

	partPath := partPathFor(dstPath)
	statePath := statePathFor(dstPath)

	state, err := loadDownloadState(statePath)
	switch {
	case os.IsNotExist(err):
		if _, statErr := os.Stat(partPath); statErr == nil {
			if err := os.Remove(partPath); err != nil {
				return nil, 0, err
			}
		}
		newState := resetDownloadState(downloadURL, expectedSize, segmentSize)
		return &newState, 0, nil
	case err != nil:
		return nil, 0, err
	}

	if state.URL != "" && state.URL != downloadURL {
		if err := cleanupPartialDownload(dstPath); err != nil {
			return nil, 0, err
		}
		newState := resetDownloadState(downloadURL, expectedSize, segmentSize)
		return &newState, 0, nil
	}
	if expectedSize > 0 && state.ExpectedSize > 0 && state.ExpectedSize != expectedSize {
		if err := cleanupPartialDownload(dstPath); err != nil {
			return nil, 0, err
		}
		newState := resetDownloadState(downloadURL, expectedSize, segmentSize)
		return &newState, 0, nil
	}

	if _, statErr := os.Stat(partPath); statErr != nil {
		if os.IsNotExist(statErr) {
			if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
				return nil, 0, err
			}
			newState := resetDownloadState(downloadURL, expectedSize, segmentSize)
			return &newState, 0, nil
		}
		return nil, 0, statErr
	}

	state.URL = downloadURL
	state.SegmentSize = normalizeSegmentSize(segmentSize)
	if expectedSize > 0 {
		state.ExpectedSize = expectedSize
	}

	verifiedOffset, changed, err := verifyPartialFile(partPath, &state)
	if err != nil {
		return nil, 0, err
	}
	if changed {
		if err := saveDownloadState(statePath, state); err != nil {
			return nil, 0, err
		}
	}
	return &state, verifiedOffset, nil
}

func verifyPartialFile(partPath string, state *downloadState) (int64, bool, error) {
	file, err := os.OpenFile(partPath, os.O_RDWR, 0)
	if err != nil {
		return 0, false, err
	}
	defer file.Close()

	if state.SegmentSize <= 0 {
		state.SegmentSize = defaultDownloadSegmentSize
	}

	validOffset := int64(0)
	trimmedSegments := state.Segments[:0]
	buf := make([]byte, state.SegmentSize)
	changed := false

	for _, segment := range state.Segments {
		if segment.Offset != validOffset || segment.Size <= 0 || segment.Size > state.SegmentSize {
			changed = true
			break
		}
		if _, err := file.Seek(segment.Offset, io.SeekStart); err != nil {
			return 0, false, err
		}
		if _, err := io.ReadFull(file, buf[:segment.Size]); err != nil {
			if err := file.Truncate(validOffset); err != nil {
				return 0, false, err
			}
			changed = true
			break
		}
		if hashSegmentHex(buf[:segment.Size]) != segment.SHA256 {
			if err := file.Truncate(validOffset); err != nil {
				return 0, false, err
			}
			changed = true
			break
		}
		trimmedSegments = append(trimmedSegments, segment)
		validOffset += segment.Size
	}

	state.Segments = trimmedSegments
	if state.PartialSegment != nil {
		segment := *state.PartialSegment
		if segment.Offset != validOffset || segment.Size <= 0 || segment.Size > state.SegmentSize {
			state.PartialSegment = nil
			state.VerifiedOffset = validOffset
			return validOffset, true, nil
		}
		if int64(len(buf)) < segment.Size {
			buf = make([]byte, segment.Size)
		}
		if _, err := file.Seek(segment.Offset, io.SeekStart); err != nil {
			return 0, false, err
		}
		if _, err := io.ReadFull(file, buf[:segment.Size]); err != nil {
			if err := file.Truncate(validOffset); err != nil {
				return 0, false, err
			}
			state.PartialSegment = nil
			state.VerifiedOffset = validOffset
			return validOffset, true, nil
		}
		if hashSegmentHex(buf[:segment.Size]) != segment.SHA256 {
			if err := file.Truncate(validOffset); err != nil {
				return 0, false, err
			}
			state.PartialSegment = nil
			state.VerifiedOffset = validOffset
			return validOffset, true, nil
		}
		validOffset += segment.Size
	}

	info, err := file.Stat()
	if err != nil {
		return 0, false, err
	}
	if info.Size() != validOffset {
		if err := file.Truncate(validOffset); err != nil {
			return 0, false, err
		}
		changed = true
	}

	state.VerifiedOffset = validOffset
	return validOffset, changed, nil
}

func newResumeStateWriter(file *os.File, state *downloadState, statePath string) (*resumeStateWriter, error) {
	writer := &resumeStateWriter{
		file:        file,
		state:       state,
		statePath:   statePath,
		segmentSize: state.SegmentSize,
		pending:     make([]byte, 0, state.SegmentSize),
	}
	if state.PartialSegment != nil {
		pending := make([]byte, state.PartialSegment.Size)
		if _, err := file.ReadAt(pending, state.PartialSegment.Offset); err != nil {
			return nil, err
		}
		writer.pending = pending
		writer.pendingAt = state.PartialSegment.Offset
	} else {
		writer.pendingAt = state.VerifiedOffset
	}
	return writer, nil
}

func (w *resumeStateWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if n > 0 {
		if writeErr := w.record(p[:n], false); writeErr != nil {
			return n, writeErr
		}
	}
	return n, err
}

func (w *resumeStateWriter) Finalize() error {
	return w.record(nil, true)
}

func (w *resumeStateWriter) record(p []byte, finalize bool) error {
	for len(p) > 0 {
		remaining := int(w.segmentSize) - len(w.pending)
		if remaining > len(p) {
			remaining = len(p)
		}
		w.pending = append(w.pending, p[:remaining]...)
		p = p[remaining:]
		if int64(len(w.pending)) == w.segmentSize {
			if err := w.persistPending(); err != nil {
				return err
			}
		}
	}
	if !finalize && len(w.pending) > 0 {
		if err := w.savePartialPending(); err != nil {
			return err
		}
	}
	if finalize && len(w.pending) > 0 {
		if err := w.persistPending(); err != nil {
			return err
		}
	}
	return nil
}

func (w *resumeStateWriter) persistPending() error {
	if len(w.pending) == 0 {
		w.state.PartialSegment = nil
		return nil
	}
	segment := downloadSegment{
		Offset: w.pendingAt,
		Size:   int64(len(w.pending)),
		SHA256: hashSegmentHex(w.pending),
	}
	w.state.PartialSegment = &segment
	w.state.VerifiedOffset = segment.Offset + segment.Size
	if int64(len(w.pending)) == w.segmentSize {
		w.state.Segments = append(w.state.Segments, segment)
		w.state.PartialSegment = nil
		w.pending = w.pending[:0]
		w.pendingAt = w.state.VerifiedOffset
	}
	return saveDownloadState(w.statePath, *w.state)
}

func (w *resumeStateWriter) savePartialPending() error {
	segment := downloadSegment{
		Offset: w.pendingAt,
		Size:   int64(len(w.pending)),
		SHA256: hashSegmentHex(w.pending),
	}
	w.state.PartialSegment = &segment
	w.state.VerifiedOffset = segment.Offset + segment.Size
	return saveDownloadState(w.statePath, *w.state)
}

func finalFileSizeMatches(path string, expectedSize int64) error {
	if expectedSize <= 0 {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() != expectedSize {
		return fmt.Errorf("download size mismatch: got %d, want %d", info.Size(), expectedSize)
	}
	return nil
}
