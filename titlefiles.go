package wiiudownloader

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	fstfmt "github.com/Xpl0itU/WiiUDownloader/internal/formats/fst"
)

// TitleFile is one file listed by a title's FST, with enough information to
// extract it on its own.
type TitleFile struct {
	// Path is the file's path inside the title, e.g. "content/foo.bar".
	Path string
	Size uint64
	// ContentID indexes the TMD's content list, not the content's own ID.
	ContentID uint16
	// Offset is the file's start inside that content.
	Offset uint64
	Length uint64
	Hashed bool
	// Shared entries carry no extractable payload, as the extractor already
	// assumes; they are listed but never downloaded.
	Shared bool
}

// TitleFileTree is a title's parsed FST plus the small working set needed to
// extract files from it. Close removes that working set.
type TitleFileTree struct {
	TitleID uint64
	Name    string
	Version int
	Files   []TitleFile

	workDir  string
	contents []Content
	cipher   cipher.Block
	client   *http.Client
}

// Close removes the temporary directory holding the metadata and FST content.
func (t *TitleFileTree) Close() error {
	if t == nil || t.workDir == "" {
		return nil
	}
	return os.RemoveAll(t.workDir)
}

// TotalSize is the sum of the listed files, excluding shared entries.
func (t *TitleFileTree) TotalSize() uint64 {
	var total uint64
	for _, file := range t.Files {
		if file.Shared {
			continue
		}
		total += file.Size
	}
	return total
}

// FetchTitleFileTree downloads a title's FST and returns every file it lists.
// The FST lives in content index 0, which is small (tens to hundreds of KB) on
// real titles, so this costs far less than the title itself.
func FetchTitleFileTree(titleID uint64, version int, client *http.Client) (tree *TitleFileTree, err error) {
	tidStr := fmt.Sprintf("%016x", titleID)
	tEntry := GetTitleEntryFromTid(titleID)

	workDir, err := os.MkdirTemp("", "wiiu-titlefiles-")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(workDir)
		}
	}()

	baseURL := fmt.Sprintf("http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%s", tidStr)

	tmdPath := filepath.Join(workDir, "title.tmd")
	tmdURL := baseURL + "/tmd"
	if version >= 0 {
		tmdURL = fmt.Sprintf("%s/tmd.%d", baseURL, version)
	}
	if err = downloadFileWithOptions(context.Background(), nil, client, tmdURL, tmdPath, downloadOptions{
		DoRetries:   true,
		AllowResume: true,
		UserAgent:   "WiiUDownloader",
		Validate: func(path string) error {
			return validateTMDFile(path, titleID)
		},
	}); err != nil {
		return nil, err
	}

	tmdData, err := os.ReadFile(tmdPath)
	if err != nil {
		return nil, err
	}
	tmd, err := ParseTMD(tmdData)
	if err != nil {
		return nil, err
	}
	if len(tmd.Contents) == 0 {
		return nil, fmt.Errorf("title %s lists no contents", tidStr)
	}
	for i := range tmd.Contents {
		tmd.Contents[i].CIDStr = fmt.Sprintf("%08X", tmd.Contents[i].ID)
	}

	if err = ensureTitleTicket(nil, client, baseURL, filepath.Join(workDir, "title.tik"), tmd, titleID, tidStr, tEntry); err != nil {
		return nil, err
	}
	titleCipher, err := newTitleCipher(filepath.Join(workDir, "title.tik"), tmd)
	if err != nil {
		return nil, err
	}

	fstContent := tmd.Contents[0]
	fstPath := filepath.Join(workDir, fstContent.CIDStr+".app")
	if err = downloadContentFile(context.Background(), nil, client, baseURL, workDir, fstContent); err != nil {
		return nil, err
	}

	fstFile, err := os.Open(fstPath)
	if err != nil {
		return nil, err
	}
	buffer := getDecryptedContentBuffer()
	defer putDecryptedContentBuffer(buffer)
	decryptErr := decryptContentToBuffer(fstFile, buffer, titleCipher, fstContent)
	fstFile.Close()
	if decryptErr != nil {
		return nil, decryptErr
	}

	table, err := fstfmt.Parse(buffer.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to parse the title's file list: %w", err)
	}

	name := tEntry.Name
	if name == "" {
		name = tidStr
	}
	files, err := flattenFST(table, tmd)
	if err != nil {
		return nil, err
	}
	return &TitleFileTree{
		TitleID:  titleID,
		Name:     name,
		Version:  version,
		Files:    files,
		workDir:  workDir,
		contents: tmd.Contents,
		cipher:   titleCipher,
		client:   client,
	}, nil
}

// downloadContentFile fetches one content (and its .h3 when hashed) into dir.
func downloadContentFile(ctx context.Context, progressReporter ProgressReporter, client *http.Client, baseURL, dir string, content Content) error {
	opts := downloadOptions{
		ExpectedSize: expectedContentDownloadSize(content),
		DoRetries:    true,
		AllowResume:  true,
		UserAgent:    "WiiUDownloader",
	}
	if err := downloadFileWithOptions(ctx, progressReporter, client, fmt.Sprintf("%s/%s", baseURL, content.CIDStr), filepath.Join(dir, content.CIDStr+".app"), opts); err != nil {
		return err
	}
	if content.Type&CONTENT_TYPE_HASHED == 0 {
		return nil
	}
	return downloadFileWithOptions(ctx, progressReporter, client, fmt.Sprintf("%s/%s.h3", baseURL, content.CIDStr), filepath.Join(dir, content.CIDStr+".h3"), downloadOptions{
		ExpectedSize: expectedH3DownloadSize(content),
		DoRetries:    true,
		AllowResume:  true,
		UserAgent:    "WiiUDownloader",
		Validate: func(path string) error {
			return verifyH3File(path, content)
		},
	})
}

// flattenFST walks the FST's directory hierarchy into file paths.
func flattenFST(table *fstfmt.Table, tmd *TMD) ([]TitleFile, error) {
	files := make([]TitleFile, 0, len(table.Entries))
	nameStack := make([]string, 0, MAX_LEVELS)
	positions := make([]uint32, MAX_LEVELS)
	level := uint32(0)
	entriesLen := uint32(len(table.Entries))

	for i := uint32(1); i < entriesLen; i++ {
		for level >= 1 && table.Entries[positions[level-1]].Length == i {
			level--
			nameStack = nameStack[:len(nameStack)-1]
		}

		current := table.Entries[i]
		name, err := table.NameAt(current.NameOffset & FST_NAME_OFFSET_MASK)
		if err != nil {
			return nil, fmt.Errorf("failed to read FST entry name: %w", err)
		}

		if current.Type&FST_DIRECTORY_TYPE_FLAG != 0 {
			if level >= MAX_LEVELS {
				return nil, errors.New("FST directory nesting exceeds limit")
			}
			positions[level] = i
			level++
			nameStack = append(nameStack, name)
			continue
		}

		offset := uint64(current.Offset)
		if current.Flags&FST_CONTENT_FACTOR_FLAG == 0 {
			offset *= uint64(table.Factor)
		}
		hashed := false
		if int(current.ContentID) < len(tmd.Contents) {
			hashed = tmd.Contents[int(current.ContentID)].Type&CONTENT_TYPE_HASHED != 0
		}
		files = append(files, TitleFile{
			Path:      strings.Join(append(append([]string{}, nameStack...), name), "/"),
			Size:      uint64(current.Length),
			ContentID: current.ContentID,
			Offset:    offset,
			Length:    uint64(current.Length),
			Hashed:    hashed,
			Shared:    current.Type&FST_SHARED_CONTENT_FLAG != 0,
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// DownloadFiles fetches the contents the selected paths need, then extracts
// exactly those files under outputDir, keeping the FST's directory tree.
func (t *TitleFileTree) DownloadFiles(outputDir string, paths []string, progressReporter ProgressReporter) error {
	if t == nil {
		return errors.New("no title file tree")
	}
	wanted := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		wanted[path] = struct{}{}
	}

	selected := make([]TitleFile, 0, len(paths))
	needed := make(map[uint16]struct{})
	for _, file := range t.Files {
		if file.Shared {
			continue
		}
		if _, ok := wanted[file.Path]; !ok {
			continue
		}
		selected = append(selected, file)
		needed[file.ContentID] = struct{}{}
	}
	if len(selected) == 0 {
		return nil
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	if progressReporter != nil {
		progressReporter.ResetTotals()
	}

	baseURL := fmt.Sprintf("http://ccs.cdn.c.shop.nintendowifi.net/ccs/download/%016x", t.TitleID)
	var missing []Content
	var totalSize int64
	for index := range needed {
		if int(index) >= len(t.contents) {
			return fmt.Errorf("invalid content index %d", index)
		}
		content := t.contents[index]
		if _, err := os.Stat(t.contentPath(index)); err == nil {
			continue
		}
		missing = append(missing, content)
		totalSize += expectedContentDownloadSize(content) + expectedH3DownloadSize(content)
	}
	if progressReporter != nil {
		progressReporter.SetDownloadSize(totalSize)
	}

	sort.Slice(missing, func(i, j int) bool { return missing[i].ID < missing[j].ID })
	for _, content := range missing {
		if progressReporter != nil && progressReporter.Cancelled() {
			return nil
		}
		if err := downloadContentFile(context.Background(), progressReporter, t.client, baseURL, t.workDir, content); err != nil {
			return err
		}
	}

	for i, file := range selected {
		if progressReporter != nil {
			if progressReporter.Cancelled() {
				return nil
			}
			progressReporter.UpdateDecryptionProgress(float64(i) / float64(len(selected)))
		}
		if err := t.extractFile(file, outputDir); err != nil {
			return err
		}
	}
	if progressReporter != nil {
		progressReporter.UpdateDecryptionProgress(1)
	}
	return nil
}

func (t *TitleFileTree) contentPath(index uint16) string {
	return filepath.Join(t.workDir, t.contents[index].CIDStr+".app")
}

func (t *TitleFileTree) extractFile(file TitleFile, outputDir string) error {
	if int(file.ContentID) >= len(t.contents) {
		return fmt.Errorf("invalid content index %d", file.ContentID)
	}
	targetPath, err := safeJoinUnderBase(outputDir, outputDir, file.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", file.Path, err)
	}

	src, err := os.Open(t.contentPath(file.ContentID))
	if err != nil {
		return err
	}
	defer src.Close()

	if file.Hashed {
		err = extractFileHash(src, 0, file.Offset, file.Length, targetPath, file.ContentID, t.cipher)
	} else {
		err = extractFile(src, 0, file.Offset, file.Length, targetPath, file.ContentID, t.cipher)
	}
	if err != nil {
		return fmt.Errorf("failed to extract %s: %w", file.Path, err)
	}
	return nil
}
