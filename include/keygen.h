#pragma once

#include <stdbool.h>
#include <stdint.h>
#include <gtitles.h>

#define getTidHighFromTid(tid) ((uint32_t)(tid >> 32))

typedef enum
{
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

bool generateKey(const char *tid, TITLE_KEY title_key, uint8_t *out);
void getTitleKeyFromTitleID(const char *tid, char *out);
int char2int(char input);
void hex2bytes(const char* input, uint8_t* output);