#include <algorithm>
#include <cstdio>
#include <cstring>
#include <gtitles.h>
#include <utils.h>

static bool isAllowedInFilename(char c) {
    return c >= ' ' && c <= '~' && c != '/' && c != '\\' && c != '"' && c != '*' && c != ':' && c != '<' && c != '>' && c != '?' && c != '|';
}

static void normalizeFilename(const char *input, char *output) {
    char ret[255];
    for (int i = 0; i < strlen(input); ++i)
        ret[i] = isAllowedInFilename(input[i]) ? input[i] : '_';
    sprintf(output, "%s", ret);
}

bool getTitleNameFromTid(uint64_t tid, char *out) {
    const TitleEntry *entries = getTitleEntries(TITLE_CATEGORY_ALL);
    const TitleEntry *found = std::find_if(entries, entries + getTitleEntriesSize(TITLE_CATEGORY_ALL), [&](const TitleEntry &e) { return e.tid == tid; });
    if (found != entries + getTitleEntriesSize(TITLE_CATEGORY_ALL)) {
        char name[255];
        normalizeFilename(found->name, name);
        sprintf(out, "%s [%016llx]", name, found->tid);
        return true;
    } else {
        sprintf(out, "Unknown");
    }
    return false;
}