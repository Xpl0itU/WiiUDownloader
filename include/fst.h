#pragma once

#include <tmd.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct FST_Header {
    uint32_t magic; // "FST\0" in ascii
    uint32_t fileOffsetMultiplier;
    int32_t numSecondaryHeaders;
    char unknown[20]; // mostly padding
} FST_Header;

typedef struct FST_SecondaryHeader {
    uint32_t offset;
    uint32_t size;
    uint64_t ownerTitleID;
    uint32_t groupID;
    uint16_t flags;
    char unknown[10];
} FST_SecondaryHeader;

typedef struct FST_FileDirEntry {
    char type;
    char nameOffset[3]; // uint24_t
    uint32_t offset;
    uint32_t size;
    uint16_t flags;
    uint16_t secondaryHeaderIndex;
} FST_FileDirEntry;

bool decryptFST(const char *path, uint8_t *outputBuffer, TMD *tmd, uint8_t *titleKey);
bool validateFST(uint8_t *data);

#ifdef __cplusplus
}
#endif