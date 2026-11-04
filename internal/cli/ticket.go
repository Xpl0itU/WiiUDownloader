package cli

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

// RunTicket handles the "ticket" subcommand.
func RunTicket(args []string) int {
	fs := flag.NewFlagSet("ticket", flag.ContinueOnError)
	var (
		outputDir string
		version   int
	)
	fs.StringVar(&outputDir, "o", "", "Output directory for title.tik and title.cert")
	fs.StringVar(&outputDir, "output", "", "Output directory for title.tik and title.cert")
	fs.IntVar(&version, "v", wiiudownloader.VersionLatest, "Title version (when providing Title ID)")
	fs.IntVar(&version, "version", wiiudownloader.VersionLatest, "Title version (when providing Title ID)")

	if err := fs.Parse(ReorderArgs(args, nil)); err != nil {
		return 1
	}

	target := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if target == "" {
		fmt.Fprintln(os.Stderr, "Error: title ID or path to title.tmd required")
		fmt.Fprintln(os.Stderr, "Usage: wiiudownloader ticket [options] <title_id | path_to_tmd>")
		return 1
	}

	if err := EnsureTitleDatabase(false); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load database: %v\n", err)
	}

	client := wiiudownloader.BuildHTTPClient()
	var tmd *wiiudownloader.TMD
	var outPath string

	// Check if target is a file
	if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
		tmdData, err := os.ReadFile(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading TMD file: %v\n", err)
			return 1
		}
		tmd, err = wiiudownloader.ParseTMD(tmdData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing TMD: %v\n", err)
			return 1
		}
		if outputDir != "" {
			outPath = outputDir
		} else {
			outPath = filepath.Dir(target)
		}
	} else {
		// Treat as Title ID
		tid, err := wiiudownloader.ParseTitleID(target)
		if err != nil || tid == 0 {
			fmt.Fprintf(os.Stderr, "Error: invalid Title ID or file %q\n", target)
			return 1
		}
		fmt.Printf("Fetching TMD for %016x from CDN... ", tid)
		tmd, err = wiiudownloader.FetchTitleTMD(tid, version, client)
		if err != nil {
			fmt.Printf("failed!\n")
			fmt.Fprintf(os.Stderr, "Error fetching TMD: %v\n", err)
			return 1
		}
		fmt.Println("done!")
		if outputDir != "" {
			outPath = outputDir
		} else {
			outPath = "."
		}
	}

	if err := os.MkdirAll(outPath, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		return 1
	}

	tidStr := fmt.Sprintf("%016x", tmd.TitleID)
	tEntry := wiiudownloader.GetTitleEntryFromTid(tmd.TitleID)
	keyType := uint8(wiiudownloader.TITLE_KEY_mypass)
	if tEntry.TitleID == tmd.TitleID {
		keyType = tEntry.Key
	}

	titleKey, err := wiiudownloader.GenerateKeyWithType(tidStr, keyType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating title key: %v\n", err)
		return 1
	}

	tikFile := filepath.Join(outPath, "title.tik")
	certFile := filepath.Join(outPath, "title.cert")

	if err := wiiudownloader.GenerateTicket(tikFile, tmd.TitleID, titleKey, tmd.TitleVersion); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing ticket: %v\n", err)
		return 1
	}

	if err := wiiudownloader.GenerateCert(tmd, certFile, nil, http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing cert: %v\n", err)
		return 1
	}

	fmt.Printf("Successfully generated:\n  - %s\n  - %s\n", tikFile, certFile)
	return 0
}
