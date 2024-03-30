package wiiudownloader

import (
	"strconv"
)

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
	titleEntries := make([]TitleEntry, 0)
	for _, entry := range titleEntry {
		if entry.Category == category {
			titleEntries = append(titleEntries, entry)
		}
	}
	return titleEntries
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

func getTitleEntryFromTid(tid string) TitleEntry {
	titleID, err := strconv.ParseUint(tid, 16, 64)
	if err != nil {
		return TitleEntry{}
	}
	for _, entry := range GetTitleEntries(TITLE_CATEGORY_ALL) {
		if entry.TitleID == titleID {
			return entry
		}
	}
	return TitleEntry{}
}
