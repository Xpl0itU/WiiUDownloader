#pragma once

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef enum MCPRegion {
    MCP_REGION_JAPAN = 0x01,
    MCP_REGION_USA = 0x02,
    MCP_REGION_EUROPE = 0x04,
    MCP_REGION_CHINA = 0x10,
    MCP_REGION_KOREA = 0x20,
    MCP_REGION_TAIWAN = 0x40,
} MCPRegion;

typedef enum {
    TID_HIGH_GAME = 0x00050000,
    TID_HIGH_DEMO = 0x00050002,
    TID_HIGH_SYSTEM_APP = 0x00050010,
    TID_HIGH_SYSTEM_DATA = 0x0005001B,
    TID_HIGH_SYSTEM_APPLET = 0x00050030,
    TID_HIGH_VWII_IOS = 0x00000007,
    TID_HIGH_VWII_SYSTEM_APP = 0x00070002,
    TID_HIGH_VWII_SYSTEM = 0x00070008,
    TID_HIGH_DLC = 0x0005000C,
    TID_HIGH_UPDATE = 0x0005000E,
} TID_HIGH;

#define getTidHighFromTid(tid) ((uint32_t) (tid >> 32))
#define isGame(tid)            (getTidHighFromTid(tid) == TID_HIGH_GAME)
#define isDLC(tid)             (getTidHighFromTid(tid) == TID_HIGH_DLC)
#define isUpdate(tid)          (getTidHighFromTid(tid) == TID_HIGH_UPDATE)

const char *getFormattedKind(uint64_t tid);
const char *getFormattedRegion(MCPRegion region);

#ifdef __cplusplus
}
#endif