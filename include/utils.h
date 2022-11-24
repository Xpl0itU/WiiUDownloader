#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

bool getTitleNameFromTid(uint64_t tid, char *out);
bool getUpdateFromBaseGame(uint64_t titleID, uint64_t *out);

#define BSWAP_8(x) ((x) &0xff)

inline uint16_t bswap_16(uint16_t value) {
    return (uint16_t) ((0x00FF & (value >> 8)) | (0xFF00 & (value << 8)));
}

inline uint32_t bswap_32(uint32_t __x) {
    return __x >> 24 | __x >> 8 & 0xff00 | __x << 8 & 0xff0000 | __x << 24;
}

inline uint64_t bswap_64(uint64_t x) {
    return (((x & 0xff00000000000000ull) >> 56) | ((x & 0x00ff000000000000ull) >> 40) | ((x & 0x0000ff0000000000ull) >> 24) | ((x & 0x000000ff00000000ull) >> 8) | ((x & 0x00000000ff000000ull) << 8) | ((x & 0x0000000000ff0000ull) << 24) | ((x & 0x000000000000ff00ull) << 40) | ((x & 0x00000000000000ffull) << 56));
}

#ifdef __cplusplus
}
#endif