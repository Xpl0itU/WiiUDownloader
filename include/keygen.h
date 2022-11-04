#pragma once

#include <stdbool.h>
#include <stdint.h>
#include <gtitles.h>

#ifdef __cplusplus
extern "C"
{
#endif

    bool generateKey(const char *tid, char *out);
    void getTitleKeyFromTitleID(const char *tid, char *out);
    int char2int(char input);
    void hex2bytes(const char* input, uint8_t* output);

#ifdef __cplusplus
}
#endif
