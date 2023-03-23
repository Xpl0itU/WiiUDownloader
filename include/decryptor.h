#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#include <stdbool.h>

void decryptor(const char *path, bool showProgressDialog, bool deleteEncryptedContents);

#ifdef __cplusplus
}
#endif