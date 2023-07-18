/***************************************************************************
 * This file is part of NUSspli.                                           *
 * Copyright (c) 2022 V10lator <v10lator@myway.de>                         *
 *                                                                         *
 * This program is free software; you can redistribute it and/or modify    *
 * it under the terms of the GNU General Public License as published by    *
 * the Free Software Foundation; either version 3 of the License, or       *
 * (at your option) any later version.                                     *
 *                                                                         *
 * This program is distributed in the hope that it will be useful,         *
 * but WITHOUT ANY WARRANTY; without even the implied warranty of          *
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the           *
 * GNU General Public License for more details.                            *
 *                                                                         *
 * You should have received a copy of the GNU General Public License along *
 * with this program; if not, If not, see <http://www.gnu.org/licenses/>.  *
 ***************************************************************************/

#pragma once

#include <stddef.h>
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

typedef enum {
    TITLE_CATEGORY_GAME = 0,
    TITLE_CATEGORY_UPDATE = 1,
    TITLE_CATEGORY_DLC = 2,
    TITLE_CATEGORY_DEMO = 3,
    TITLE_CATEGORY_ALL = 4,
    TITLE_CATEGORY_DISC = 5,
} TITLE_CATEGORY;

typedef enum {
    TITLE_KEY_mypass = 0,
    TITLE_KEY_nintendo = 1,
    TITLE_KEY_test = 2,
    TITLE_KEY_1234567890 = 3,
    TITLE_KEY_Lucy131211 = 4,
    TITLE_KEY_fbf10 = 5,
    TITLE_KEY_5678 = 6,
    TITLE_KEY_1234 = 7,
    TITLE_KEY_ = 8,
    TITLE_KEY_MAGIC = 9,
} TITLE_KEY;

typedef struct
{
    const char *name;
    const uint64_t tid;
    const MCPRegion region;
    const TITLE_KEY key;
} TitleEntry;

const TitleEntry *getTitleEntries(TITLE_CATEGORY cat);
size_t getTitleEntriesSize(TITLE_CATEGORY cat);

#ifdef __cplusplus
}
#endif
