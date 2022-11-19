#pragma once

#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

bool getTitleNameFromTid(uint64_t tid, char *out);

#ifdef __cplusplus
}
#endif