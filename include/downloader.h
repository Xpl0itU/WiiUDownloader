#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

void downloadTitle(const char *titleID, bool decrypt);

#ifdef __cplusplus
}
#endif