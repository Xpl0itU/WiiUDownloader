#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

void generateHashes(const char *path);
bool compareHashes(const char *h3hashPath);

#ifdef __cplusplus
}
#endif