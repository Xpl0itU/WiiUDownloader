package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

type app struct {
	input       *bufio.Reader
	stdout      io.Writer
	stderr      io.Writer
	interactive bool
	client      *http.Client
	interrupts  *interruptHandler

	dbPath    string
	noDBFetch bool
	quiet     bool
	json      bool

	db *titleDB
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	application := &app{
		input:       bufio.NewReader(os.Stdin),
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		interactive: isTerminal(os.Stdin),
		client:      buildHTTPClient(),
	}
	application.interrupts = newInterruptHandler(application.stderr)
	defer application.interrupts.stop()

	global := flag.NewFlagSet(cliName, flag.ContinueOnError)
	global.SetOutput(application.stderr)
	bindCommonFlags(global, application)
	var showVersion bool
	global.BoolVar(&showVersion, "version", false, "Print the CLI version")
	global.Usage = func() { printGeneralUsage(application.stderr) }
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if showVersion {
		fmt.Fprintf(application.stdout, "%s %s\n", cliName, cliVersion)
		return 0
	}

	rest := global.Args()
	if len(rest) == 0 {
		printGeneralUsage(application.stdout)
		return 0
	}

	command, commandArgs := rest[0], rest[1:]
	var err error
	switch command {
	case "search", "list":
		err = cmdSearch(application, commandArgs)
	case "download":
		err = cmdDownload(application, commandArgs)
	case "info":
		err = cmdInfo(application, commandArgs)
	case "files":
		err = cmdFiles(application, commandArgs)
	case "extract":
		err = cmdExtract(application, commandArgs)
	case "decrypt":
		err = cmdDecrypt(application, commandArgs)
	case "db":
		err = cmdDB(application, commandArgs)
	case "version":
		fmt.Fprintf(application.stdout, "%s %s\n", cliName, cliVersion)
		return 0
	case "help":
		return runHelp(application, commandArgs)
	default:
		fmt.Fprintf(application.stderr, "%s: unknown command %q\n\n", cliName, command)
		printGeneralUsage(application.stderr)
		return 2
	}

	if err == nil {
		return 0
	}
	var silent silentExit
	if errors.As(err, &silent) {
		return silent.code
	}
	fmt.Fprintf(application.stderr, "%s: %v\n", cliName, err)
	var usage usageError
	if errors.As(err, &usage) {
		return 2
	}
	var interrupted interruptedError
	if errors.As(err, &interrupted) {
		return 130
	}
	return 1
}

func runHelp(application *app, args []string) int {
	if len(args) == 0 {
		printGeneralUsage(application.stdout)
		return 0
	}
	// Command help belongs on stdout when it was explicitly requested.
	originalStderr := application.stderr
	application.stderr = application.stdout
	defer func() { application.stderr = originalStderr }()
	command := args[0]
	switch command {
	case "search", "list":
		return exitCode(cmdSearch(application, []string{"-h"}))
	case "download":
		return exitCode(cmdDownload(application, []string{"-h"}))
	case "info":
		return exitCode(cmdInfo(application, []string{"-h"}))
	case "files":
		return exitCode(cmdFiles(application, []string{"-h"}))
	case "extract":
		return exitCode(cmdExtract(application, []string{"-h"}))
	case "decrypt":
		return exitCode(cmdDecrypt(application, []string{"-h"}))
	case "db":
		fmt.Fprintln(application.stdout, dbUsage)
		return 0
	default:
		fmt.Fprintf(application.stderr, "%s: unknown command %q\n", cliName, command)
		return 2
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var silent silentExit
	if errors.As(err, &silent) {
		return silent.code
	}
	return 1
}

func bindCommonFlags(fs *flag.FlagSet, application *app) {
	fs.StringVar(&application.dbPath, "db", application.dbPath, "Title database file (JSON or generated Go source)")
	fs.BoolVar(&application.noDBFetch, "no-db-fetch", application.noDBFetch, "Never download the title database automatically")
	fs.BoolVar(&application.quiet, "quiet", application.quiet, "Suppress progress output")
	fs.BoolVar(&application.quiet, "q", application.quiet, "Alias for --quiet")
	fs.BoolVar(&application.json, "json", application.json, "Emit JSON")
}

func newFlagSet(application *app, name, summary, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(application.stderr)
	bindCommonFlags(fs, application)
	fs.Usage = func() {
		fmt.Fprintln(application.stderr, summary)
		fmt.Fprintln(application.stderr)
		fmt.Fprintln(application.stderr, usage)
		fs.PrintDefaults()
	}
	return fs
}

func flagError(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return silentExit{code: 2}
}

// parseInterspersed parses flags that appear before and after positional
// arguments, which is what users expect from a CLI even though the stdlib flag
// package stops at the first non-flag token.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
	return positional, nil
}

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }

type silentExit struct{ code int }

func (e silentExit) Error() string { return "" }

type interruptedError struct{}

func (interruptedError) Error() string { return "interrupted" }

func (a *app) warnf(format string, args ...any) {
	fmt.Fprintf(a.stderr, "warning: "+format+"\n", args...)
}

func (a *app) loadDB() (*titleDB, error) {
	if a.db != nil {
		return a.db, nil
	}
	db, err := loadTitleDatabase(a.client, titleDBOptions{
		path:    a.dbPath,
		noFetch: a.noDBFetch,
		warn:    a.warnf,
	})
	if err != nil {
		return nil, err
	}
	a.db = db
	return db, nil
}

// loadDBForTitles allows download/info/files/extract to keep working from bare
// title IDs when the database is unavailable, but still requires the database
// when a name has to be resolved.
func (a *app) loadDBForTitles(arguments []string) (*titleDB, error) {
	db, err := a.loadDB()
	if err == nil {
		return db, nil
	}
	if a.dbPath != "" {
		return nil, err
	}
	for _, argument := range arguments {
		if _, ok := parseTitleID(argument); !ok {
			return nil, err
		}
	}
	a.warnf("title database unavailable (%v); continuing with title IDs", err)
	db = newTitleDB(nil, "unavailable")
	db.setGlobal()
	a.db = db
	return db, nil
}

func printGeneralUsage(w io.Writer) {
	fmt.Fprintf(w, `%s %s - download Wii U titles from Nintendo's CDN

Usage:
  %s [global flags] <command> [flags] [args]

Commands:
  search [query...]       Search the title database (no query lists everything)
  info <title...>         Show metadata and CDN details for titles
  files <title>           List the files inside a title without downloading it
  download <title...>     Download one or more titles
  extract <title>         Download and extract selected files from a title
  decrypt <dir...>        Decrypt previously downloaded title folders
  db <update|info|clear>  Manage the title database cache
  version                 Print the CLI version
  help [command]          Show help for the CLI or a command

A <title> is a 16-digit hex title ID (00050000101c9500 or 0x00050000101C9500)
or a name/query. Ambiguous names prompt for a selection on a terminal.

Global flags:
  --db PATH          Use a title database file (JSON or generated Go source)
  --no-db-fetch      Never download the title database automatically
  --quiet            Suppress progress output
  --json             Emit machine-readable JSON (where supported)

The title database is downloaded once and cached in your user cache directory.
Set WIIUD_DB_URL to use a different source.

Examples:
  %s search zelda
  %s search --region usa --type game --limit 10 mario
  %s info 00050000101c9500
  %s download --decrypt -o ~/Games "Breath of the Wild"
  %s files 00050000101c9500
  %s extract 00050000101c9500 --file content/...
  %s decrypt ~/Games/Game [Game] [00050000101c9500]
  %s db update
`, cliName, cliVersion, cliName, cliName, cliName, cliName, cliName, cliName, cliName, cliName, cliName)
}

var dbUsage = `Usage:
  wiiudl db update [--url URL]  Download and cache the latest title database
  wiiudl db info                Show which title database is in use
  wiiudl db clear               Delete the cached title database`
