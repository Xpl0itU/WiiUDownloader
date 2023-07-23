#pragma once

typedef void (*ProgressCallback)(int progress);
void set_progress_callback(ProgressCallback cb);
int cdecrypt_main(int argc, char **argv);
