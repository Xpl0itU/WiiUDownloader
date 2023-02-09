#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

bool saveSettings(const char *selectedDir, bool hideWiiVCWarning);
bool loadSettings();

#ifdef __cplusplus
}
#endif