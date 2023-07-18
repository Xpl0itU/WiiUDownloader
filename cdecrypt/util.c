/*
  Common code for Gust (Koei/Tecmo) PC games tools
  Copyright © 2019-2021 VitaSmith

  This program is free software: you can redistribute it and/or modify
  it under the terms of the GNU General Public License as published by
  the Free Software Foundation, either version 3 of the License, or
  (at your option) any later version.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
  GNU General Public License for more details.

  You should have received a copy of the GNU General Public License
  along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "utf8.h"
#include "util.h"

bool create_path(char *path) {
  bool result = true;
  struct stat64_t st;
#if defined(_WIN32)
  // Ignore Windows drive names
  if ((strlen(path) == 2) && (path[1] == ':'))
    return true;
#endif
  if (stat64_utf8(path, &st) != 0) {
    // Directory doesn't exist, create it
    size_t pos = 0;
    for (size_t n = strlen(path); n > 0; n--) {
      if (path[n] == PATH_SEP) {
        while ((n > 0) && (path[--n] == PATH_SEP))
          ;
        pos = n + 1;
        break;
      }
    }
    if (pos > 0) {
      // Create parent dirs
      path[pos] = 0;
      char *new_path = malloc(strlen(path) + 1);
      if (new_path == NULL) {
        fprintf(stderr, "ERROR: Can't allocate path\n");
        return false;
      }
      strcpy(new_path, path);
      result = create_path(new_path);
      free(new_path);
      path[pos] = PATH_SEP;
    }
    // Create node
    if (result)
      result = CREATE_DIR(path);
  } else if (!S_ISDIR(st.st_mode)) {
    fprintf(stderr, "ERROR: '%s' exists but isn't a directory\n", path);
    return false;
  }

  return result;
}

// dirname/basename, that *PRESERVE* the string parameter.
// Note that these calls are not concurrent, meaning that you MUST be done
// using the returned string from a previous call before invoking again.
#if defined(_WIN32)
char *_basename_win32(const char *path, bool remove_extension) {
  static char basename[128];
  static char ext[64];
  ext[0] = 0;
  _splitpath_s(path, NULL, 0, NULL, 0, basename, sizeof(basename), ext,
               sizeof(ext));
  if ((ext[0] != 0) && !remove_extension)
    strncat(basename, ext, sizeof(basename) - strlen(basename));
  return basename;
}

// This call should behave pretty similar to UNIX' dirname
char *_dirname_win32(const char *path) {
  static char dir[PATH_MAX];
  static char drive[4];
  int found_sep = 0;
  memset(drive, 0, sizeof(drive));
  _splitpath_s(path, drive, sizeof(drive), dir, sizeof(dir) - 3, NULL, 0, NULL,
               0);
  // Only deal with drives that are one letter
  drive[2] = 0;
  drive[3] = 0;
  if (drive[1] != ':')
    drive[0] = 0;
  // Removing trailing path separators
  for (int32_t n = (int32_t)strlen(dir) - 1;
       (n > 0) && ((dir[n] == '/') || (dir[n] == '\\')); n--) {
    dir[n] = 0;
    found_sep++;
  }
  if (dir[0] == 0) {
    if (drive[0] == 0)
      return found_sep ? "\\" : ".";
    drive[2] = '\\';
    return drive;
  }
  if (drive[0] != 0) {
    // Add the drive
    memmove(&dir[2], dir, strlen(dir) + 1);
    memcpy(dir, drive, strlen(drive));
    dir[2] = '\\';
  }
  return dir;
}
#else
char *_basename_unix(const char *path) {
  static char path_copy[PATH_MAX];
  strncpy(path_copy, path, sizeof(path_copy));
  path_copy[PATH_MAX - 1] = 0;
  return basename(path_copy);
}

char *_dirname_unix(const char *path) {
  static char path_copy[PATH_MAX];
  strncpy(path_copy, path, sizeof(path_copy));
  path_copy[PATH_MAX - 1] = 0;
  return dirname(path_copy);
}
#endif

bool is_file(const char *path) {
  struct stat64_t st;
  return (stat64_utf8(path, &st) == 0) && S_ISREG(st.st_mode);
}

bool is_directory(const char *path) {
  struct stat64_t st;
  return (stat64_utf8(path, &st) == 0) && S_ISDIR(st.st_mode);
}

char *change_extension(const char *path, const char *extension) {
  static char new_path[PATH_MAX];
  strncpy(new_path, _basename((char *)path), sizeof(new_path) - 1);
  for (size_t i = 0; i < sizeof(new_path); i++) {
    if (new_path[i] == '.')
      new_path[i] = 0;
  }
  strncat(new_path, extension, sizeof(new_path) - strlen(new_path) - 1);
  return new_path;
}

size_t get_trailing_slash(const char *path) {
  size_t i;
  if ((path == NULL) || (path[0] == 0))
    return 0;
  for (i = strlen(path) - 1; (i > 0) && ((path[i] != '/') && (path[i] != '\\'));
       i--)
    ;
  return (i == 0) ? 0 : i + 1;
}

uint32_t read_file_max(const char *path, uint8_t **buf, uint32_t max_size) {
  FILE *file = fopen_utf8(path, "rb");
  if (file == NULL) {
    fprintf(stderr, "ERROR: Can't open '%s'\n", path);
    return 0;
  }

  fseek(file, 0L, SEEK_END);
  uint32_t size = (uint32_t)ftell(file);
  fseek(file, 0L, SEEK_SET);
  if (max_size != 0)
    size = min(size, max_size);

  *buf = calloc(size, 1);
  if (*buf == NULL) {
    size = 0;
    goto out;
  }
  if (fread(*buf, 1, size, file) != size) {
    fprintf(stderr, "ERROR: Can't read '%s'\n", path);
    size = 0;
  }
out:
  fclose(file);
  if (size == 0) {
    free(*buf);
    *buf = NULL;
  }
  return size;
}

uint64_t get_file_size(const char *path) {
  FILE *file = fopen_utf8(path, "rb");
  if (file == NULL) {
    fprintf(stderr, "ERROR: Can't open '%s'\n", path);
    return 0;
  }

  fseek(file, 0L, SEEK_END);
  uint64_t size = ftell64(file);
  fclose(file);
  return size;
}

void create_backup(const char *path) {
  struct stat64_t st;
  if (stat64_utf8(path, &st) == 0) {
    char *backup_path = malloc(strlen(path) + 5);
    if (backup_path == NULL)
      return;
    strcpy(backup_path, path);
    strcat(backup_path, ".bak");
    if (stat64_utf8(backup_path, &st) != 0) {
      if (rename_utf8(path, backup_path) == 0)
        printf("Saved backup as '%s'\n", backup_path);
      else
        fprintf(stderr, "WARNING: Could not create backup file '%s\n",
                backup_path);
    }
    free(backup_path);
  }
}

bool write_file(const uint8_t *buf, const uint32_t size, const char *path,
                const bool backup) {
  if (backup)
    create_backup(path);
  FILE *file = fopen_utf8(path, "wb");
  if (file == NULL) {
    fprintf(stderr, "ERROR: Can't create file '%s'\n", path);
    return false;
  }
  bool r = (fwrite(buf, 1, size, file) == size);
  fclose(file);
  if (!r)
    fprintf(stderr, "ERROR: Can't write file '%s'\n", path);
  return r;
}
