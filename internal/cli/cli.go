package cli

import (
	"fmt"
	"os"
	"strings"
)

const (
	CLIVersion = "3.2"
)

var knownCommands = map[string]struct{}{
	"download":  {},
	"d":         {},
	"search":    {},
	"s":         {},
	"list":      {},
	"ls":        {},
	"info":      {},
	"decrypt":   {},
	"ticket":    {},
	"files":     {},
	"queue":     {},
	"db":        {},
	"help":      {},
	"--help":    {},
	"-h":        {},
	"version":   {},
	"--version": {},
}

// IsCLICommand checks if args represent a CLI invocation rather than a GUI launch.
func IsCLICommand(args []string) bool {
	if len(args) <= 1 {
		return false
	}
	cmd := strings.ToLower(strings.TrimSpace(args[1]))
	if _, ok := knownCommands[cmd]; ok {
		return true
	}
	return false
}

// Run executes the CLI with the provided arguments and returns an exit code.
func Run(args []string) int {
	if len(args) == 0 {
		PrintUsage()
		return 0
	}

	cmd := strings.ToLower(strings.TrimSpace(args[0]))
	subArgs := args[1:]

	switch cmd {
	case "download", "d":
		return RunDownload(subArgs)
	case "search", "s":
		return RunSearch(subArgs)
	case "list", "ls":
		return RunList(subArgs)
	case "info":
		return RunInfo(subArgs)
	case "decrypt":
		return RunDecrypt(subArgs)
	case "ticket":
		return RunTicket(subArgs)
	case "files":
		return RunFiles(subArgs)
	case "queue":
		return RunQueue(subArgs)
	case "db":
		return RunDB(subArgs)
	case "version", "--version", "-v":
		fmt.Printf("WiiUDownloader CLI v%s\n", CLIVersion)
		return 0
	case "help", "--help", "-h":
		PrintUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %q\n\n", cmd)
		PrintUsage()
		return 1
	}
}

// ReorderArgs moves flags before positional arguments so that Go's flag.FlagSet
// can parse them even if the user placed flags after positional arguments.
func ReorderArgs(args []string, boolFlags map[string]bool) []string {
	var flags []string
	var positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			flagName := strings.TrimLeft(arg, "-")
			// If flag has "=value", no next arg is needed
			if strings.Contains(flagName, "=") {
				continue
			}
			// If it's a known boolean flag, it doesn't consume the next arg
			if boolFlags != nil && boolFlags[flagName] {
				continue
			}
			// If the next token exists and is not a flag, it is the flag's value
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			positionals = append(positionals, arg)
		}
	}
	return append(flags, positionals...)
}

// PrintUsage prints the general CLI help message.
func PrintUsage() {
	fmt.Printf("WiiUDownloader CLI v%s\n", CLIVersion)
	fmt.Println("Download, decrypt, and manage Wii U games, updates, DLCs, and demos directly from Nintendo's CDN.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  wiiudownloader <command> [options] [arguments]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  download, d   Download titles (games, updates, DLCs) by Title ID or name")
	fmt.Println("  search, s     Search for titles in the database by name or Title ID")
	fmt.Println("  list, ls      List titles from the database with region and category filters")
	fmt.Println("  info          Show title details, contents count, total size, and related DLC/updates")
	fmt.Println("  decrypt       Decrypt previously downloaded title folder(s)")
	fmt.Println("  ticket        Generate fake ticket (title.tik) and cert (title.cert) for a title or TMD")
	fmt.Println("  files         List files inside a title FST without downloading full game")
	fmt.Println("  queue         Inspect or download titles saved in WiiUDownloader GUI queue")
	fmt.Println("  db            Manage local title database (stats, update)")
	fmt.Println("  version       Show version information")
	fmt.Println("  help          Show this help message")
	fmt.Println()
	fmt.Println("Download Examples:")
	fmt.Println("  wiiudownloader download 0005000010143500 -d")
	fmt.Println("      Download and decrypt Mario Kart 8 (USA)")
	fmt.Println()
	fmt.Println("  wiiudownloader download 0005000010143500 -d --all")
	fmt.Println("      Download and decrypt game + latest update + DLC")
	fmt.Println()
	fmt.Println("  wiiudownloader download --file titles.txt -o /path/to/cemu/games -d")
	fmt.Println("      Batch download and decrypt titles listed in a file")
	fmt.Println()
	fmt.Println("  wiiudownloader download --from-queue -d --delete-encrypted")
	fmt.Println("      Download items queued in the WiiUDownloader GUI")
	fmt.Println()
	fmt.Println("  wiiudownloader decrypt /path/to/downloaded/title --delete-encrypted")
	fmt.Println("      Decrypt an already downloaded game folder")
	fmt.Println()
	fmt.Println("Run 'wiiudownloader <command> --help' for details on a specific command.")
}
