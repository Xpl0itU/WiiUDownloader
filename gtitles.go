package wiiudownloader

import "strings"

const (
	MCP_REGION_JAPAN  = 0x01
	MCP_REGION_USA    = 0x02
	MCP_REGION_EUROPE = 0x04
	MCP_REGION_CHINA  = 0x10
	MCP_REGION_KOREA  = 0x20
	MCP_REGION_TAIWAN = 0x40
)

const (
	TITLE_KEY_mypass = iota
	TITLE_KEY_nintendo
	TITLE_KEY_test
	TITLE_KEY_1234567890
	TITLE_KEY_Lucy131211
	TITLE_KEY_fbf10
	TITLE_KEY_5678
	TITLE_KEY_1234
	TITLE_KEY_
	TITLE_KEY_MAGIC
)

const (
	TITLE_CATEGORY_GAME = iota
	TITLE_CATEGORY_UPDATE
	TITLE_CATEGORY_DLC
	TITLE_CATEGORY_DEMO
	TITLE_CATEGORY_ALL
	TITLE_CATEGORY_DISC
)

const (
	TID_HIGH_GAME            = 0x00050000
	TID_HIGH_DEMO            = 0x00050002
	TID_HIGH_SYSTEM_APP      = 0x00050010
	TID_HIGH_SYSTEM_DATA     = 0x0005001B
	TID_HIGH_SYSTEM_APPLET   = 0x00050030
	TID_HIGH_VWII_IOS        = 0x00000007
	TID_HIGH_VWII_SYSTEM_APP = 0x00070002
	TID_HIGH_VWII_SYSTEM     = 0x00070008
	TID_HIGH_DLC             = 0x0005000C
	TID_HIGH_UPDATE          = 0x0005000E
)

func GetTitleEntries(category uint8) []TitleEntry {
	titleEntries := make([]TitleEntry, 0, len(titleEntry))
	for _, entry := range titleEntry {
		if entry.Category == TITLE_CATEGORY_DISC {
			continue
		}
		if category == TITLE_CATEGORY_ALL || category == entry.Category {
			titleEntries = append(titleEntries, entry)
		}
	}
	return titleEntries
}

func GetFormattedRegion(region uint8) string {
	var regions []string
	if region&MCP_REGION_USA != 0 {
		regions = append(regions, "USA")
	}
	if region&MCP_REGION_EUROPE != 0 {
		regions = append(regions, "Europe")
	}
	if region&MCP_REGION_JAPAN != 0 {
		regions = append(regions, "Japan")
	}

	switch len(regions) {
	case 0:
		return "Unknown"
	case 3:
		return "All"
	default:
		return strings.Join(regions, "/")
	}
}

func GetFormattedKind(titleID uint64) string {
	switch titleID >> 32 {
	case TID_HIGH_GAME:
		return "Game"
	case TID_HIGH_DEMO:
		return "Demo"
	case TID_HIGH_SYSTEM_APP:
		return "System App"
	case TID_HIGH_SYSTEM_DATA:
		return "System Data"
	case TID_HIGH_SYSTEM_APPLET:
		return "System Applet"
	case TID_HIGH_VWII_IOS:
		return "vWii IOS"
	case TID_HIGH_VWII_SYSTEM_APP:
		return "vWii System App"
	case TID_HIGH_VWII_SYSTEM:
		return "vWii System"
	case TID_HIGH_DLC:
		return "DLC"
	case TID_HIGH_UPDATE:
		return "Update"
	default:
		return "Unknown"
	}
}

func GetCategoryFromFormattedCategory(formattedCategory string) uint8 {
	switch formattedCategory {
	case "Game":
		return TITLE_CATEGORY_GAME
	case "Update":
		return TITLE_CATEGORY_UPDATE
	case "DLC":
		return TITLE_CATEGORY_DLC
	case "Demo":
		return TITLE_CATEGORY_DEMO
	case "All":
		return TITLE_CATEGORY_ALL
	default:
		return TITLE_CATEGORY_ALL
	}
}

func GetTitleIDHigh(tid uint64) uint32 {
	return uint32(tid >> 32)
}

func GetTitleIDLow(tid uint64) uint32 {
	return uint32(tid & 0xFFFFFFFF)
}

func GetRelatedTypeTargets(high uint32) []uint32 {
	switch high {
	case TID_HIGH_GAME:
		return []uint32{TID_HIGH_DLC, TID_HIGH_UPDATE}
	case TID_HIGH_DLC:
		return []uint32{TID_HIGH_GAME, TID_HIGH_UPDATE}
	case TID_HIGH_UPDATE:
		return []uint32{TID_HIGH_GAME, TID_HIGH_DLC}
	default:
		return nil
	}
}

func FindRelatedTitleByHighAndLow(source TitleEntry, targetHigh uint32, exclude map[uint64]struct{}) (TitleEntry, bool) {
	sourceLow := GetTitleIDLow(source.TitleID)

	var best TitleEntry
	bestScore := 3
	found := false

	for _, entry := range titleEntry {
		if entry.Category == TITLE_CATEGORY_DISC {
			continue
		}
		if GetTitleIDHigh(entry.TitleID) != targetHigh {
			continue
		}
		if GetTitleIDLow(entry.TitleID) != sourceLow {
			continue
		}
		if exclude != nil {
			if _, skip := exclude[entry.TitleID]; skip {
				continue
			}
		}

		score := 2
		if entry.Region == source.Region {
			score = 0
		} else if entry.Region&source.Region != 0 {
			score = 1
		}

		if !found || score < bestScore || (score == bestScore && entry.TitleID < best.TitleID) {
			best = entry
			bestScore = score
			found = true
		}
	}

	return best, found
}

func GetTitleEntryFromTid(tid uint64) TitleEntry {
	for _, entry := range titleEntry {
		if entry.Category == TITLE_CATEGORY_DISC {
			continue
		}
		if entry.TitleID == tid {
			return entry
		}
	}
	return TitleEntry{}
}
