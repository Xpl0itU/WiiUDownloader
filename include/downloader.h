#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

void downloadTitle(const char *titleID, bool decrypt, bool *cancelQueue);

#ifdef __cplusplus
}
#endif