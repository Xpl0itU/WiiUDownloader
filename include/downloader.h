#pragma once

#include <stdbool.h>
#include <gtk/gtk.h>

#ifdef __cplusplus
extern "C" {
#endif

void progressDialog();
void destroyProgressDialog();
GtkWidget *getProgressBar();
void setSelectedDir(const char *path);
char *getSelectedDir();
void setHideWiiVCWarning(bool value);
bool getHideWiiVCWarning();
void setQueueCancelled(bool value);
int downloadTitle(const char *titleID, const char *name, bool decrypt, bool deleteEncryptedContents);

#ifdef __cplusplus
}
#endif