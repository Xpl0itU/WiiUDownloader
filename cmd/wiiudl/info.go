package main

import (
	"fmt"
	"text/tabwriter"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type contentInfoJSON struct {
	ID     string `json:"id"`
	Size   int64  `json:"size"`
	Hashed bool   `json:"hashed"`
}

type titleInfoJSON struct {
	Name             string            `json:"name"`
	TitleID          string            `json:"titleId"`
	Kind             string            `json:"kind"`
	Region           string            `json:"region"`
	Category         string            `json:"category"`
	DatabaseVersion  int               `json:"databaseVersion,omitempty"`
	RequestedVersion string            `json:"requestedVersion"`
	TitleVersion     *int              `json:"titleVersion,omitempty"`
	ContentCount     *int              `json:"contentCount,omitempty"`
	DownloadSize     *int64            `json:"downloadSize,omitempty"`
	Contents         []contentInfoJSON `json:"contents,omitempty"`
	TMDError         string            `json:"tmdError,omitempty"`
}

func cmdInfo(application *app, args []string) error {
	fs := newFlagSet(application, "info", "Show metadata and CDN details for one or more titles.", `Usage:
  wiiudl info [flags] <title...>

Flags:`)
	version := fs.String("version", "latest", "Title version to inspect: latest or a non-negative number")
	offline := fs.Bool("offline", false, "Do not contact the CDN (only show database metadata)")
	contents := fs.Bool("contents", false, "List every content in the TMD")
	first := fs.Bool("first", false, "Pick the first name match instead of prompting when ambiguous")
	region := fs.String("region", "all", "Regions used when resolving names: usa,eur,jpn,chn,kor,twn,all")
	category := fs.String("type", "all", "Content type used when resolving names: game, update, dlc, demo or all")
	positional, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}
	if len(positional) == 0 {
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

	results := make([]titleInfoJSON, 0, len(positional))
	for index, argument := range positional {
		entry, err := resolveOneTitle(application, db, argument, regionMask, categoryValue, *first)
		if err != nil {
			return err
		}
		titleVersion, err := parseTitleVersion(*version, entry)
		if err != nil {
			return err
		}

		info := titleInfoJSON{
			Name:             entry.Name,
			TitleID:          fmt.Sprintf("%016x", entry.TitleID),
			Kind:             wiiudownloader.GetFormattedKind(entry.TitleID),
			Region:           formatRegion(entry.Region),
			Category:         categoryName(categoryOf(entry)),
			DatabaseVersion:  entry.Version,
			RequestedVersion: displayVersion(titleVersion),
		}

		if !*offline {
			tmd, tmdErr := wiiudownloader.FetchTitleTMD(entry.TitleID, titleVersion, application.client)
			if tmdErr != nil {
				info.TMDError = tmdErr.Error()
				if !application.quiet {
					fmt.Fprintf(application.stderr, "warning: %s: could not fetch TMD: %v\n", displayName(entry), tmdErr)
				}
			} else {
				titleVersionValue := int(tmd.TitleVersion)
				contentCount := len(tmd.Contents)
				size := int64(tmd.CalculateTotalSize())
				info.TitleVersion = &titleVersionValue
				info.ContentCount = &contentCount
				info.DownloadSize = &size
				if *contents {
					info.Contents = make([]contentInfoJSON, 0, len(tmd.Contents))
					for _, content := range tmd.Contents {
						info.Contents = append(info.Contents, contentInfoJSON{
							ID:     fmt.Sprintf("%08x", content.ID),
							Size:   content.DownloadSize(),
							Hashed: content.Type&wiiudownloader.CONTENT_TYPE_HASHED != 0,
						})
					}
				}
			}
		}

		results = append(results, info)

		if application.json {
			continue
		}
		if index > 0 {
			fmt.Fprintln(application.stdout)
		}
		printTitleInfo(application, info, *contents)
	}

	if application.json {
		return writeJSON(application.stdout, results)
	}
	return nil
}

func printTitleInfo(application *app, info titleInfoJSON, showContents bool) {
	fmt.Fprintf(application.stdout, "Name:           %s\n", valueOrUnknown(info.Name))
	fmt.Fprintf(application.stdout, "Title ID:       %s\n", info.TitleID)
	fmt.Fprintf(application.stdout, "Kind:           %s\n", info.Kind)
	fmt.Fprintf(application.stdout, "Region:         %s\n", info.Region)
	fmt.Fprintf(application.stdout, "Category:       %s\n", info.Category)
	if info.DatabaseVersion > 0 {
		fmt.Fprintf(application.stdout, "DB version:     %d\n", info.DatabaseVersion)
	}
	fmt.Fprintf(application.stdout, "Requested:      %s\n", info.RequestedVersion)
	if info.TMDError != "" {
		fmt.Fprintf(application.stdout, "TMD:            unavailable (%s)\n", info.TMDError)
	} else if info.TitleVersion != nil {
		fmt.Fprintf(application.stdout, "Title version:  %d\n", *info.TitleVersion)
		fmt.Fprintf(application.stdout, "Contents:       %d\n", *info.ContentCount)
		fmt.Fprintf(application.stdout, "Download size:  %s\n", formatBytes(*info.DownloadSize))
	}
	if showContents && len(info.Contents) > 0 {
		fmt.Fprintln(application.stdout)
		writer := tabwriter.NewWriter(application.stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "CONTENT ID\tSIZE\tHASHED")
		for _, content := range info.Contents {
			hashed := "no"
			if content.Hashed {
				hashed = "yes"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\n", content.ID, formatBytes(content.Size), hashed)
		}
		writer.Flush()
	}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "(not in database)"
	}
	return value
}
