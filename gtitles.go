package wiiudownloader

/*
#cgo CFLAGS: -I${SRCDIR}/gtitles
#cgo LDFLAGS: -Wl,-rpath,${SRCDIR}/gtitles
#cgo LDFLAGS: -L${SRCDIR}/gtitles
#cgo LDFLAGS: -lgtitles
#include <gtitles.h>
#include <ctype.h>
*/
import "C"
import "unsafe"

const (
	MCP_REGION_JAPAN  = 0x01
	MCP_REGION_USA    = 0x02
	MCP_REGION_EUROPE = 0x04
	MCP_REGION_CHINA  = 0x10
	MCP_REGION_KOREA  = 0x20
	MCP_REGION_TAIWAN = 0x40
)

const (
	TITLE_KEY_mypass     = 0
	TITLE_KEY_nintendo   = 1
	TITLE_KEY_test       = 2
	TITLE_KEY_1234567890 = 3
	TITLE_KEY_Lucy131211 = 4
	TITLE_KEY_fbf10      = 5
	TITLE_KEY_5678       = 6
	TITLE_KEY_1234       = 7
	TITLE_KEY_           = 8
	TITLE_KEY_MAGIC      = 9
)

const (
	TITLE_CATEGORY_GAME   = 0
	TITLE_CATEGORY_UPDATE = 1
	TITLE_CATEGORY_DLC    = 2
	TITLE_CATEGORY_DEMO   = 3
	TITLE_CATEGORY_ALL    = 4
	TITLE_CATEGORY_DISC   = 5
)

type TitleEntry struct {
	Name    string
	TitleID uint64
	Region  uint8
	key     uint8
}

func GetTitleEntries(category uint8) []TitleEntry {
	entriesSize := getTitleEntriesSize(category)
	entriesSlice := make([]TitleEntry, entriesSize)
	cEntries := C.getTitleEntries(C.TITLE_CATEGORY(category))
	cSlice := (*[1 << 28]C.TitleEntry)(unsafe.Pointer(cEntries))[:entriesSize:entriesSize]
	for i := 0; i < entriesSize; i++ {
		entriesSlice[i] = TitleEntry{
			Name:    C.GoString(cSlice[i].name),
			TitleID: uint64(cSlice[i].tid),
			Region:  uint8(cSlice[i].region),
			key:     uint8(cSlice[i].key),
		}
	}
	return entriesSlice
}

func getTitleEntriesSize(category uint8) int {
	return int(C.getTitleEntriesSize(C.TITLE_CATEGORY(category)))
}

func GetFormattedRegion(region uint8) string {
	if region&MCP_REGION_EUROPE != 0 {
		if region&MCP_REGION_USA != 0 {
			if region&MCP_REGION_JAPAN != 0 {
				return "All"
			}
			return "USA/Europe"
		}
		if region&MCP_REGION_JAPAN != 0 {
			return "Europe/Japan"
		}
		return "Europe"
	}
	if region&MCP_REGION_USA != 0 {
		if region&MCP_REGION_JAPAN != 0 {
			return "USA/Japan"
		}
		return "USA"
	}
	if region&MCP_REGION_JAPAN != 0 {
		return "Japan"
	}
	return "Unknown"
}
