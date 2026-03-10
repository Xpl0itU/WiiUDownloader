package wiiudownloader

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	defaultDownloadSegmentSize = 1 << 20
	mediumDownloadSegmentSize  = 4 << 20
	largeDownloadSegmentSize   = 16 << 20
	maxSmallDownloadSize       = 64 << 20
	maxMediumDownloadSize      = 1 << 30

	downloadPartExtension  = ".part"
	downloadStateExtension = ".resume.json"
	downloadStateDirPerm   = 0o755
	downloadFilePerm       = 0o644
	resumeJournalMagic     = "WUDRESUME2"
)

var resumeJournalHeader = []byte(resumeJournalMagic + "\n")

var errInvalidDownloadState = errors.New("invalid download state")

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

func loadDownloadState(path string) (downloadState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return downloadState{}, false, err
	}
	if bytes.HasPrefix(data, resumeJournalHeader) {
		return loadDownloadStateJournal(data)
	}
	return loadLegacyDownloadState(data)
}

func loadLegacyDownloadState(data []byte) (downloadState, bool, error) {
	var state downloadState
	if err := json.Unmarshal(data, &state); err != nil {
		return downloadState{}, false, fmt.Errorf("%w: %v", errInvalidDownloadState, err)
	}
	state.SegmentSize = normalizeSegmentSize(state.SegmentSize, state.ExpectedSize)
	return state, false, nil
}

func loadDownloadStateJournal(data []byte) (downloadState, bool, error) {
	lines := bytes.Split(data[len(resumeJournalHeader):], []byte{'\n'})
	var (
		state downloadState
		found bool
		dirty bool
	)

	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var snapshot downloadState
		if err := json.Unmarshal(line, &snapshot); err != nil {
			if found {
				dirty = true
				break
			}
			return downloadState{}, false, fmt.Errorf("%w: %v", errInvalidDownloadState, err)
		}
		snapshot.SegmentSize = normalizeSegmentSize(snapshot.SegmentSize, snapshot.ExpectedSize)
		state = snapshot
		found = true
	}

	if !found {
		return downloadState{}, false, fmt.Errorf("%w: download state journal is empty", errInvalidDownloadState)
	}
	return state, dirty, nil
}

func saveDownloadState(path string, state downloadState) error {
	return persistDownloadState(path, state, false)
}

func rewriteDownloadState(path string, state downloadState) error {
	return persistDownloadState(path, state, true)
}

func persistDownloadState(path string, state downloadState, rewrite bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, downloadStateDirPerm); err != nil {
		return err
	}

	state.SegmentSize = normalizeSegmentSize(state.SegmentSize, state.ExpectedSize)
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	snapshot := append(data, '\n')

	if rewrite {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, downloadFilePerm)
		if err != nil {
			return err
		}
		if _, err := file.Write(resumeJournalHeader); err != nil {
			file.Close()
			return err
		}
		if _, err := file.Write(snapshot); err != nil {
			file.Close()
			return err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return err
		}
		return file.Close()
	}

	hasHeader, err := resumeStateHasJournalHeader(path)
	if err != nil {
		return err
	}
	if !hasHeader {
		return persistDownloadState(path, state, true)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, downloadFilePerm)
	if err != nil {
		return err
	}
	if _, err := file.Write(snapshot); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func resumeStateHasJournalHeader(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()

	header := make([]byte, len(resumeJournalHeader))
	if _, err := io.ReadFull(file, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		return false, err
	}
	return bytes.Equal(header, resumeJournalHeader), nil
}

func normalizeSegmentSize(segmentSize int64, expectedSize int64) int64 {
	if segmentSize > 0 {
		return segmentSize
	}
	switch {
	case expectedSize > 0 && expectedSize <= defaultDownloadSegmentSize:
		return expectedSize
	case expectedSize > 0 && expectedSize <= maxSmallDownloadSize:
		return defaultDownloadSegmentSize
	case expectedSize > 0 && expectedSize <= maxMediumDownloadSize:
		return mediumDownloadSegmentSize
	case expectedSize > 0:
		return largeDownloadSegmentSize
	default:
		return defaultDownloadSegmentSize
	}
}

func resetDownloadState(downloadURL string, expectedSize int64, segmentSize int64) downloadState {
	return downloadState{
		URL:          downloadURL,
		ExpectedSize: expectedSize,
		SegmentSize:  normalizeSegmentSize(segmentSize, expectedSize),
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

	state, dirty, err := loadDownloadState(statePath)
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
		if errors.Is(err, errInvalidDownloadState) {
			if err := cleanupPartialDownload(dstPath); err != nil {
				return nil, 0, err
			}
			newState := resetDownloadState(downloadURL, expectedSize, segmentSize)
			return &newState, 0, nil
		}
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
	state.SegmentSize = normalizeSegmentSize(segmentSize, expectedSize)
	if expectedSize > 0 {
		state.ExpectedSize = expectedSize
	}

	verifiedOffset, changed, err := verifyPartialFile(partPath, &state)
	if err != nil {
		return nil, 0, err
	}
	if changed || dirty {
		if err := rewriteDownloadState(statePath, state); err != nil {
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

	state.SegmentSize = normalizeSegmentSize(state.SegmentSize, state.ExpectedSize)

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
	if err := w.reconcileFromFile(); err != nil {
		return err
	}
	return w.record(nil, true)
}

func (w *resumeStateWriter) reconcileFromFile() error {
	info, err := w.file.Stat()
	if err != nil {
		return err
	}
	recordedOffset := w.state.VerifiedOffset
	if len(w.pending) > 0 {
		recordedOffset = w.pendingAt + int64(len(w.pending))
	}
	if info.Size() <= recordedOffset {
		return nil
	}

	missing := make([]byte, info.Size()-recordedOffset)
	if _, err := w.file.ReadAt(missing, recordedOffset); err != nil {
		return err
	}
	return w.record(missing, false)
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
	if finalize && len(w.pending) > 0 {
		if err := w.savePartialPending(); err != nil {
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
	w.state.Segments = append(w.state.Segments, segment)
	w.state.PartialSegment = nil
	w.state.VerifiedOffset = segment.Offset + segment.Size
	w.pending = w.pending[:0]
	w.pendingAt = w.state.VerifiedOffset

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
