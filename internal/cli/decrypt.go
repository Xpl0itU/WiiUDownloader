package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

// RunDecrypt handles the "decrypt" subcommand.
func RunDecrypt(args []string) int {
	fs := flag.NewFlagSet("decrypt", flag.ContinueOnError)
	var (
		outputDir       string
		deleteEncrypted bool
		quiet           bool
	)
	fs.StringVar(&outputDir, "o", "", "Separate output directory for decrypted files (default: decrypt in-place)")
	fs.StringVar(&outputDir, "output", "", "Separate output directory for decrypted files (default: decrypt in-place)")
	fs.BoolVar(&deleteEncrypted, "delete-encrypted", false, "Delete encrypted files (.app, .h3, etc.) after successful decryption")
	fs.BoolVar(&quiet, "q", false, "Quiet mode: suppress progress output")
	fs.BoolVar(&quiet, "quiet", false, "Quiet mode: suppress progress output")

	boolFlags := map[string]bool{"delete-encrypted": true, "q": true, "quiet": true}
	if err := fs.Parse(ReorderArgs(args, boolFlags)); err != nil {
		return 1
	}

	targets := fs.Args()
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "Error: at least one directory to decrypt is required")
		fmt.Fprintln(os.Stderr, "Usage: wiiudownloader decrypt [options] <folder1> [folder2...]")
		return 1
	}

	// Discover folders with title.tmd
	var foldersToDecrypt []string
	for _, target := range targets {
		if fi, err := os.Stat(target); err != nil || !fi.IsDir() {
			fmt.Fprintf(os.Stderr, "Skipping invalid directory: %s\n", target)
			continue
		}

		tmdPath := filepath.Join(target, "title.tmd")
		if _, err := os.Stat(tmdPath); err == nil {
			foldersToDecrypt = append(foldersToDecrypt, target)
			continue
		}

		// Check one level down in case a directory containing multiple title folders was given
		entries, err := os.ReadDir(target)
		if err == nil {
			subFound := false
			for _, entry := range entries {
				if entry.IsDir() {
					subTmd := filepath.Join(target, entry.Name(), "title.tmd")
					if _, err := os.Stat(subTmd); err == nil {
						foldersToDecrypt = append(foldersToDecrypt, filepath.Join(target, entry.Name()))
						subFound = true
					}
				}
			}
			if !subFound {
				fmt.Fprintf(os.Stderr, "Skipping %s: no title.tmd found\n", target)
			}
		}
	}

	if len(foldersToDecrypt) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no valid title folders (with title.tmd) found to decrypt.")
		return 1
	}

	reporter := NewTerminalProgressReporter(quiet)
	defer reporter.Close()

	var successCount, failCount int
	for i, folder := range foldersToDecrypt {
		if reporter.Cancelled() {
			break
		}

		folderName := filepath.Base(folder)
		if !quiet {
			fmt.Printf("[%d/%d] Decrypting: %s\n", i+1, len(foldersToDecrypt), folderName)
		}

		decryptOut := ""
		if outputDir != "" {
			decryptOut = filepath.Join(outputDir, folderName)
		}

		reporter.ResetTotals()
		err := wiiudownloader.DecryptContents(folder, reporter, deleteEncrypted, decryptOut)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ Decryption failed for %s: %v\n", folderName, err)
			failCount++
		} else {
			if !quiet {
				outDest := folder
				if decryptOut != "" {
					outDest = decryptOut
				}
				fmt.Printf("  ✓ Successfully decrypted to: %s\n", outDest)
			}
			successCount++
		}
	}

	if !quiet {
		fmt.Println()
		fmt.Printf("Summary: %d decrypted successfully, %d failed.\n", successCount, failCount)
	}

	if failCount > 0 {
		return 1
	}
	return 0
}
