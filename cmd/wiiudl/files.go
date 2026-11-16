package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type fileInfoJSON struct {
	Path      string `json:"path"`
	Size      uint64 `json:"size"`
	ContentID uint16 `json:"contentId"`
	Offset    uint64 `json:"offset"`
	Hashed    bool   `json:"hashed"`
	Shared    bool   `json:"shared"`
}

type titleFilesJSON struct {
	Name      string         `json:"name"`
	TitleID   string         `json:"titleId"`
	Kind      string         `json:"kind"`
	Version   string         `json:"version"`
	TotalSize uint64         `json:"totalSize"`
	Files     []fileInfoJSON `json:"files"`
}

type extractResultJSON struct {
	Name    string   `json:"name"`
	TitleID string   `json:"titleId"`
	Version string   `json:"version"`
	Output  string   `json:"output"`
	Files   []string `json:"files"`
}

func cmdFiles(application *app, args []string) error {
	fs := newFlagSet(application, "files", "List the files inside a title without downloading the whole title.", `Usage:
  wiiudl files [flags] <title>

Only the title's FST (its file table) is downloaded, which is usually tiny.
Use "wiiudl extract" to download selected files.

Flags:`)
	version := fs.String("version", "latest", "Title version: latest or a non-negative number")
	filter := fs.String("filter", "", "Only list paths containing this text")
	first := fs.Bool("first", false, "Pick the first name match instead of prompting when ambiguous")
	region := fs.String("region", "all", "Regions used when resolving names: usa,eur,jpn,chn,kor,twn,all")
	category := fs.String("type", "all", "Content type used when resolving names: game, update, dlc, demo or all")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}
	if len(positional) != 1 {
		fs.Usage()
		return silentExit{code: 2}
	}
	regionMask, err := parseRegionMask(*region)
	if err != nil {
		return usageError{err: err}
	}
	categoryValue, err := parseCategory(*category)
	if err != nil {
		return usageError{err: err}
	}

	db, err := application.loadDBForTitles(positional)
	if err != nil {
		return err
	}
	entry, err := resolveOneTitle(application, db, positional[0], regionMask, categoryValue, *first)
	if err != nil {
		return err
	}
	titleVersion, err := parseTitleVersion(*version, entry)
	if err != nil {
		return err
	}

	if !application.quiet {
		fmt.Fprintf(application.stderr, "Fetching the file list for %s (%016x)...\n", displayName(entry), entry.TitleID)
	}
	tree, err := wiiudownloader.FetchTitleFileTree(entry.TitleID, titleVersion, application.client)
	if err != nil {
		return err
	}
	defer tree.Close()

	if len(tree.Files) == 0 {
		return fmt.Errorf("title %016x lists no files", entry.TitleID)
	}

	needle := strings.ToLower(strings.TrimSpace(*filter))
	files := make([]wiiudownloader.TitleFile, 0, len(tree.Files))
	var displayedSize uint64
	for _, file := range tree.Files {
		if needle != "" && !strings.Contains(strings.ToLower(file.Path), needle) {
			continue
		}
		files = append(files, file)
		if !file.Shared {
			displayedSize += file.Size
		}
	}

	if application.json {
		result := titleFilesJSON{
			Name:      entry.Name,
			TitleID:   fmt.Sprintf("%016x", entry.TitleID),
			Kind:      wiiudownloader.GetFormattedKind(entry.TitleID),
			Version:   displayVersion(titleVersion),
			TotalSize: displayedSize,
			Files:     make([]fileInfoJSON, 0, len(files)),
		}
		for _, file := range files {
			result.Files = append(result.Files, fileInfoJSON{
				Path:      file.Path,
				Size:      file.Size,
				ContentID: file.ContentID,
				Offset:    file.Offset,
				Hashed:    file.Hashed,
				Shared:    file.Shared,
			})
		}
		return writeJSON(application.stdout, result)
	}

	fmt.Fprintf(application.stdout, "%s (%016x), version %s\n", valueOrUnknown(entry.Name), entry.TitleID, displayVersion(titleVersion))
	if needle != "" {
		fmt.Fprintf(application.stdout, "%d of %d files, %s\n", len(files), len(tree.Files), formatBytes(int64(displayedSize)))
	} else {
		fmt.Fprintf(application.stdout, "%d files, %s\n", len(files), formatBytes(int64(displayedSize)))
	}
	fmt.Fprintln(application.stdout)

	writer := tabwriter.NewWriter(application.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "SIZE\tPATH")
	for _, file := range files {
		size := formatBytes(int64(file.Size))
		if file.Shared {
			size = "shared"
		}
		fmt.Fprintf(writer, "%s\t%s\n", size, file.Path)
	}
	return writer.Flush()
}

type stringListFlag []string

func (s *stringListFlag) String() string { return strings.Join(*s, ",") }

func (s *stringListFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func cmdExtract(application *app, args []string) error {
	fs := newFlagSet(application, "extract", "Download and extract selected files from a title.", `Usage:
  wiiudl extract [flags] <title>

Files are selected with --file PATH (repeatable), --filter TEXT or --all.
The output folder mirrors the GUI: <--output>/<name> [<kind>] [<title id>].

Flags:`)
	version := fs.String("version", "latest", "Title version: latest or a non-negative number")
	outputDir := "."
	fs.StringVar(&outputDir, "output", ".", "Parent directory for extracted files")
	fs.StringVar(&outputDir, "o", ".", "Alias for --output")
	var selectedFiles stringListFlag
	fs.Var(&selectedFiles, "file", "File path inside the title (repeatable)")
	all := fs.Bool("all", false, "Extract every file in the title")
	filter := fs.String("filter", "", "Extract every path containing this text")
	first := fs.Bool("first", false, "Pick the first name match instead of prompting when ambiguous")
	region := fs.String("region", "all", "Regions used when resolving names: usa,eur,jpn,chn,kor,twn,all")
	category := fs.String("type", "all", "Content type used when resolving names: game, update, dlc, demo or all")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}
	if len(positional) != 1 {
		fs.Usage()
		return silentExit{code: 2}
	}
	if !*all && len(selectedFiles) == 0 && strings.TrimSpace(*filter) == "" {
		return usageError{fmt.Errorf("select files with --file PATH, --filter TEXT or --all (use `%s files <title>` to list them)", cliName)}
	}
	regionMask, err := parseRegionMask(*region)
	if err != nil {
		return usageError{err: err}
	}
	categoryValue, err := parseCategory(*category)
	if err != nil {
		return usageError{err: err}
	}

	db, err := application.loadDBForTitles(positional)
	if err != nil {
		return err
	}
	entry, err := resolveOneTitle(application, db, positional[0], regionMask, categoryValue, *first)
	if err != nil {
		return err
	}
	titleVersion, err := parseTitleVersion(*version, entry)
	if err != nil {
		return err
	}

	if !application.quiet {
		fmt.Fprintf(application.stderr, "Fetching the file list for %s (%016x)...\n", displayName(entry), entry.TitleID)
	}
	tree, err := wiiudownloader.FetchTitleFileTree(entry.TitleID, titleVersion, application.client)
	if err != nil {
		return err
	}
	defer tree.Close()
	if len(tree.Files) == 0 {
		return fmt.Errorf("title %016x lists no files", entry.TitleID)
	}

	paths, err := selectTitleFiles(tree, selectedFiles, *all, *filter)
	if err != nil {
		return err
	}

	targetDir := filepath.Join(absolutePath(outputDir), titleDirName(entry, titleVersion))
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if !application.quiet {
		fmt.Fprintf(application.stderr, "Extracting %d file(s) to %s\n", len(paths), targetDir)
	}
	progress := newTerminalProgress(application.stderr, isTerminal(os.Stderr), application.quiet)
	application.interrupts.set(progress)
	runErr := tree.DownloadFiles(targetDir, paths, progress)
	progress.Finish()
	application.interrupts.set(nil)

	if progress.Cancelled() {
		return interruptedError{}
	}
	if runErr != nil {
		return runErr
	}
	if application.json {
		return writeJSON(application.stdout, extractResultJSON{
			Name:    entry.Name,
			TitleID: fmt.Sprintf("%016x", entry.TitleID),
			Version: displayVersion(titleVersion),
			Output:  targetDir,
			Files:   paths,
		})
	}
	if !application.quiet {
		fmt.Fprintf(application.stderr, "Extracted %d file(s).\n", len(paths))
	}
	return nil
}

func selectTitleFiles(tree *wiiudownloader.TitleFileTree, requested []string, all bool, filter string) ([]string, error) {
	needle := strings.ToLower(strings.TrimSpace(filter))
	byPath := make(map[string]wiiudownloader.TitleFile, len(tree.Files))
	for _, file := range tree.Files {
		byPath[file.Path] = file
	}

	selected := make(map[string]struct{})
	add := func(path string) {
		selected[path] = struct{}{}
	}

	if all {
		for _, file := range tree.Files {
			if !file.Shared {
				add(file.Path)
			}
		}
	}

	missing := make([]string, 0)
	for _, path := range requested {
		normalized := normalizeTitlePath(path)
		file, ok := byPath[normalized]
		if !ok {
			missing = append(missing, path)
			continue
		}
		if file.Shared {
			return nil, usageError{fmt.Errorf("%s is a shared entry and has no extractable payload", normalized)}
		}
		add(normalized)
	}
	if len(missing) > 0 {
		var builder strings.Builder
		fmt.Fprintf(&builder, "%d file(s) were not found in the title:\n", len(missing))
		limit := len(missing)
		if limit > 10 {
			limit = 10
		}
		for _, path := range missing[:limit] {
			fmt.Fprintf(&builder, "  %s\n", path)
		}
		if len(missing) > limit {
			builder.WriteString("  ...\n")
		}
		builder.WriteString("Use `" + cliName + " files <title>` to list the exact paths.")
		return nil, usageError{fmt.Errorf("%s", builder.String())}
	}

	if needle != "" {
		for _, file := range tree.Files {
			if file.Shared {
				continue
			}
			if strings.Contains(strings.ToLower(file.Path), needle) {
				add(file.Path)
			}
		}
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("no files selected")
	}

	// Keep the FST's path order for a stable output.
	paths := make([]string, 0, len(selected))
	for _, file := range tree.Files {
		if _, ok := selected[file.Path]; ok && !file.Shared {
			paths = append(paths, file.Path)
		}
	}
	return paths, nil
}

func normalizeTitlePath(path string) string {
	path = strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	return path
}
