#include <titleInfo.h>

const char *getFormattedKind(uint64_t tid) {
    uint32_t highID = getTidHighFromTid(tid);
    switch (highID) {
        case TID_HIGH_GAME:
            return "Game";
        case TID_HIGH_DEMO:
            return "Demo";
        case TID_HIGH_SYSTEM_APP:
            return "System App";
        case TID_HIGH_SYSTEM_DATA:
            return "System Data";
        case TID_HIGH_SYSTEM_APPLET:
            return "System Applet";
        case TID_HIGH_VWII_IOS:
            return "vWii IOS";
        case TID_HIGH_VWII_SYSTEM_APP:
            return "vWii System App";
        case TID_HIGH_VWII_SYSTEM:
            return "vWii System";
        case TID_HIGH_DLC:
            return "DLC";
        case TID_HIGH_UPDATE:
            return "Update";
        default:
            return "Unknown";
    }
}

const char *getFormattedRegion(MCPRegion region) {
    if (region & MCP_REGION_EUROPE) {
        if (region & MCP_REGION_USA)
            return region & MCP_REGION_JAPAN ? "All" : "USA/Europe";

        return region & MCP_REGION_JAPAN ? "Europe/Japan" : "Europe";
    }

    if (region & MCP_REGION_USA)
        return region & MCP_REGION_JAPAN ? "USA/Japan" : "USA";

    return region & MCP_REGION_JAPAN ? "Japan" : "Unknown";
}