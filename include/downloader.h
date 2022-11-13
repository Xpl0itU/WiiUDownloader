#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

int downloadTitle(const char *titleID, bool decrypt);

#ifdef __cplusplus
}
#endif