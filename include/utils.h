#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

bool getTitleNameFromTid(uint64_t tid, char *out);
bool getUpdateFromBaseGame(uint64_t titleID, uint64_t *out);
uint64_t getContentSize(int contentIndex, uint8_t *tmd);

#ifdef __cplusplus
}
#endif