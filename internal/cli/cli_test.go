package cli

import (
	"reflect"
	"strings"
	"testing"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

func TestReorderArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		boolFlags map[string]bool
		expected  []string
	}{
		{
			name:      "Flags already in front",
			args:      []string{"-r", "usa", "mario"},
			boolFlags: nil,
			expected:  []string{"-r", "usa", "mario"},
		},
		{
			name:      "Flags after positional arguments",
			args:      []string{"Zelda", "-r", "usa", "-t", "game"},
			boolFlags: nil,
			expected:  []string{"-r", "usa", "-t", "game", "Zelda"},
		},
		{
			name:      "Boolean flags without values",
			args:      []string{"0005000010143500", "-d", "--delete-encrypted", "-o", "/tmp/out"},
			boolFlags: map[string]bool{"d": true, "delete-encrypted": true},
			expected:  []string{"-d", "--delete-encrypted", "-o", "/tmp/out", "0005000010143500"},
		},
		{
			name:      "Flag with equals",
			args:      []string{"title", "--output=/tmp/dir"},
			boolFlags: nil,
			expected:  []string{"--output=/tmp/dir", "title"},
		},
		{
			name:      "Double dash stops flag processing",
			args:      []string{"--output", "/tmp", "--", "-not-a-flag"},
			boolFlags: nil,
			expected:  []string{"--output", "/tmp", "-not-a-flag"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ReorderArgs(tc.args, tc.boolFlags)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("ReorderArgs(%v) = %v; expected %v", tc.args, got, tc.expected)
			}
		})
	}
}

func TestParseRegionFilter(t *testing.T) {
	usa := uint8(wiiudownloader.MCP_REGION_USA)
	eur := uint8(wiiudownloader.MCP_REGION_EUROPE)
	jpn := uint8(wiiudownloader.MCP_REGION_JAPAN)

	if got := parseRegionFilter("usa"); got != usa {
		t.Errorf("expected usa (%d), got %d", usa, got)
	}
	if got := parseRegionFilter("eur"); got != eur {
		t.Errorf("expected eur (%d), got %d", eur, got)
	}
	if got := parseRegionFilter("jpn"); got != jpn {
		t.Errorf("expected jpn (%d), got %d", jpn, got)
	}
	if got := parseRegionFilter("usa,eur"); got != (usa | eur) {
		t.Errorf("expected usa|eur (%d), got %d", usa|eur, got)
	}
	if got := parseRegionFilter("all"); got != (usa | eur | jpn) {
		t.Errorf("expected all (%d), got %d", usa|eur|jpn, got)
	}
}

func TestParseCategoryFilter(t *testing.T) {
	if got := parseCategoryFilter("game"); got != wiiudownloader.TITLE_CATEGORY_GAME {
		t.Errorf("expected GAME, got %d", got)
	}
	if got := parseCategoryFilter("update"); got != wiiudownloader.TITLE_CATEGORY_UPDATE {
		t.Errorf("expected UPDATE, got %d", got)
	}
	if got := parseCategoryFilter("dlc"); got != wiiudownloader.TITLE_CATEGORY_DLC {
		t.Errorf("expected DLC, got %d", got)
	}
	if got := parseCategoryFilter("demo"); got != wiiudownloader.TITLE_CATEGORY_DEMO {
		t.Errorf("expected DEMO, got %d", got)
	}
	if got := parseCategoryFilter("all"); got != wiiudownloader.TITLE_CATEGORY_ALL {
		t.Errorf("expected ALL, got %d", got)
	}
}

func TestParseTitleDatabaseFromGo(t *testing.T) {
	sample := `
package wiiudownloader
var titleEntry = []TitleEntry{
	{Name:"Test Game 1",TitleID:0x0005000010143500,Region:2,Key:0,Category:0},
	{Name:"Test Game 2 (Special \"Quotes\")",TitleID:0x0005000e10143500,Region:4,Key:1,Category:1},
}
`
	entries, err := ParseTitleDatabaseFromGo(sample)
	if err != nil {
		t.Fatalf("unexpected error parsing sample DB: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Name != "Test Game 1" || entries[0].TitleID != 0x0005000010143500 || entries[0].Region != 2 {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].Name != `Test Game 2 (Special "Quotes")` || entries[1].Category != 1 {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}
}

func TestIsCLICommand(t *testing.T) {
	if IsCLICommand([]string{"WiiUDownloader"}) {
		t.Error("expected false for no args")
	}
	if !IsCLICommand([]string{"WiiUDownloader", "download", "0005000010143500"}) {
		t.Error("expected true for download command")
	}
	if !IsCLICommand([]string{"WiiUDownloader", "search", "mario"}) {
		t.Error("expected true for search command")
	}
	if !IsCLICommand([]string{"WiiUDownloader", "--help"}) {
		t.Error("expected true for --help")
	}
	if IsCLICommand([]string{"WiiUDownloader", "--gapplication-service"}) {
		t.Error("expected false for GTK internal flags")
	}
}

func TestRunSearchAndList(t *testing.T) {
	if err := EnsureTitleDatabase(true); err != nil {
		t.Skipf("skipping test requiring title database: %v", err)
	}

	code := RunSearch([]string{"--json", "--limit", "5", "Mario"})
	if code != 0 {
		t.Errorf("expected RunSearch exit code 0, got %d", code)
	}

	code = RunList([]string{"--json", "--limit", "5", "-t", "game"})
	if code != 0 {
		t.Errorf("expected RunList exit code 0, got %d", code)
	}
}

func TestParseTitleID(t *testing.T) {
	// Full 16-hex
	tid, err := wiiudownloader.ParseTitleID("0005000010143500")
	if err != nil || tid != 0x0005000010143500 {
		t.Errorf("expected 0x0005000010143500, got %016x, err %v", tid, err)
	}

	// With 0x prefix
	tid, err = wiiudownloader.ParseTitleID("0x0005000010143500")
	if err != nil || tid != 0x0005000010143500 {
		t.Errorf("expected 0x0005000010143500, got %016x, err %v", tid, err)
	}

	// 8-hex low ID
	tid, err = wiiudownloader.ParseTitleID("10143500")
	if err != nil || tid != 0x0005000010143500 {
		t.Errorf("expected 0x0005000010143500, got %016x, err %v", tid, err)
	}
}

func TestFormatTitleFolderName(t *testing.T) {
	name := "Super Mario 3D World"
	tid := uint64(0x0005000010145C00)
	folder := wiiudownloader.FormatTitleFolderName(name, tid, -1)
	expected := "Super Mario 3D World [Game] [0005000010145c00]"
	if !strings.EqualFold(folder, expected) {
		t.Errorf("expected %q, got %q", expected, folder)
	}

	folderWithVer := wiiudownloader.FormatTitleFolderName(name, tid, 64)
	expectedWithVer := "Super Mario 3D World [Game] [0005000010145c00] [v64]"
	if !strings.EqualFold(folderWithVer, expectedWithVer) {
		t.Errorf("expected %q, got %q", expectedWithVer, folderWithVer)
	}
}
