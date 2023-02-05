#pragma once

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

void setSelectedDir(const char *path);
int downloadTitle(const char *titleID, const char *name, bool decrypt, bool *cancelQueue, bool deleteEncryptedContents, bool showProgressDialog);

#ifdef __cplusplus
}
#endif