package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type FileItemJSON struct {
	Path   string `json:"path"`
	Size   uint64 `json:"size"`
	Shared bool   `json:"shared"`
}

type FileTreeJSON struct {
	TitleID   string         `json:"title_id"`
	Name      string         `json:"name"`
	TotalSize uint64         `json:"total_size"`
	FileCount int            `json:"file_count"`
	Files     []FileItemJSON `json:"files"`
}

// RunFiles handles the "files" subcommand.
func RunFiles(args []string) int {
	fs := flag.NewFlagSet("files", flag.ContinueOnError)
	var (
		version int
		search  string
		jsonOut bool
	)
	fs.IntVar(&version, "v", wiiudownloader.VersionLatest, "Title version (-1 for latest)")
	fs.IntVar(&version, "version", wiiudownloader.VersionLatest, "Title version (-1 for latest)")
	fs.StringVar(&search, "s", "", "Filter files by path or extension")
	fs.StringVar(&search, "search", "", "Filter files by path or extension")
	fs.BoolVar(&jsonOut, "json", false, "Output in JSON format")

	boolFlags := map[string]bool{"json": true}
	if err := fs.Parse(ReorderArgs(args, boolFlags)); err != nil {
		return 1
	}

	target := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if target == "" {
		fmt.Fprintln(os.Stderr, "Error: title ID required")
		fmt.Fprintln(os.Stderr, "Usage: wiiudownloader files [options] <title_id>")
		return 1
	}

	if err := EnsureTitleDatabase(jsonOut); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	tid, err := wiiudownloader.ParseTitleID(target)
	if err != nil || tid == 0 {
		fmt.Fprintf(os.Stderr, "Error: invalid Title ID %q\n", target)
		return 1
	}

	if !jsonOut {
		fmt.Printf("Fetching FST directory structure for %016x from CDN... ", tid)
	}

	client := wiiudownloader.BuildHTTPClient()
	tree, err := wiiudownloader.FetchTitleFileTree(tid, version, client)
	if err != nil {
		if !jsonOut {
			fmt.Printf("failed!\n")
		}
		fmt.Fprintf(os.Stderr, "Error fetching file tree: %v\n", err)
		return 1
	}
	defer tree.Close()

	if !jsonOut {
		fmt.Printf("done! (%d files)\n\n", len(tree.Files))
	}

	searchLower := strings.ToLower(search)
	var matched []wiiudownloader.TitleFile
	for _, f := range tree.Files {
		if searchLower != "" && !strings.Contains(strings.ToLower(f.Path), searchLower) {
			continue
		}
		matched = append(matched, f)
	}

	if jsonOut {
		out := FileTreeJSON{
			TitleID:   fmt.Sprintf("%016x", tree.TitleID),
			Name:      tree.Name,
			TotalSize: tree.TotalSize(),
			FileCount: len(matched),
			Files:     make([]FileItemJSON, len(matched)),
		}
		for i, f := range matched {
			out.Files[i] = FileItemJSON{
				Path:   f.Path,
				Size:   f.Size,
				Shared: f.Shared,
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	fmt.Printf("%-12s %s\n", "SIZE", "INTERNAL PATH")
	fmt.Println(strings.Repeat("-", 60))
	for _, f := range matched {
		sizeStr := wiiudownloader.FormatBytes(f.Size)
		if f.Shared {
			sizeStr = "[shared]"
		}
		fmt.Printf("%-12s %s\n", sizeStr, f.Path)
	}
	fmt.Printf("\nTotal: %d file(s), %s uncompressed\n", len(matched), wiiudownloader.FormatBytes(tree.TotalSize()))
	return 0
}
