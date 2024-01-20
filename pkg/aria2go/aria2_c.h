#ifndef ARIA2_C_H
#define ARIA2_C_H

#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/**
 * Type definition for file information in torrent.
 */
struct FileInfo {
  int index;
  const char *name;
  int64_t length;
  int64_t completedLength;
  bool selected;
};

/**
 * Type definition for BitTorrent meta information.
 */
struct MetaInfo {
  const char *name;
  const char *comment;
  int64_t creationUnix;
  const char *announceList;
};

/**
 * Type definition for download information.
 */
struct DownloadInfo {
  int status;
  int64_t totalLength;
  int64_t bytesCompleted;
  int64_t uploadLength;
  int downloadSpeed;
  int uploadSpeed;
  int pieceLength;
  int numPieces;
  int connections;
  int numFiles;
  const char *infoHash;
  struct MetaInfo *metaInfo;
  struct FileInfo *files;
  int errorCode;
  uint64_t followedByGid;
};

int init(uint64_t aria2goPointer, const char *options);
int shutdownSchedules(bool force);
int deinit();
uint64_t addUri(char *uri, const char *options);
uint64_t addTorrent(char *fp, const char *options);
bool changeOptions(uint64_t gid, const char *options);
const char *getOptions(uint64_t gid);
bool changeGlobalOptions(const char *options);
const char *getGlobalOptions();
int run();
bool pause(uint64_t gid);
bool resume(uint64_t gid);
bool removeDownload(uint64_t gid);
struct DownloadInfo *getDownloadInfo(uint64_t gid);

#ifdef __cplusplus
}
#endif

#endif