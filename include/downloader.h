#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

void downloadTitle(const char *titleID, const char *name, bool decrypt, bool *cancelQueue);

#ifdef __cplusplus
}
#endif