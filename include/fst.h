#pragma once

#include <tmd.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

struct FSTInfo {
    uint32_t Unknown;
    uint32_t Size;
    uint32_t UnknownB;
    uint32_t UnknownC[6];
};

struct FST {
    uint32_t MagicBytes;
    uint32_t Unknown;
    uint32_t EntryCount;

    uint32_t UnknownB[5];

    struct FSTInfo FSTInfos[];
};

struct FEntry {
    union {
        struct
        {
            uint32_t Type : 8;
            uint32_t NameOffset : 24;
        };
        uint32_t TypeName;
    };
    union {
        struct // File Entry
        {
            uint32_t FileOffset;
            uint32_t FileLength;
        };
        struct // Dir Entry
        {
            uint32_t ParentOffset;
            uint32_t NextOffset;
        };
        uint32_t entry[2];
    };
    uint16_t Flags;
    uint16_t ContentID;
};

bool decryptFST(const char *path, uint8_t *outputBuffer, TMD *tmd, uint8_t *titleKey);
bool validateFST(uint8_t *data);
bool containsFile(uint8_t *data, const char *path);

#ifdef __cplusplus
}
#endif