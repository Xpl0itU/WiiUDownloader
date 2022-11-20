#include <algorithm>
#include <cstdio>
#include <cstring>
#include <gtitles.h>
#include <utils.h>

static void normalizeFilename(const char* filename, char *out) {
    const char keep[] = " ._";
    size_t j = 0;
    for (size_t i = 0; filename[i]; ++i) {
        char c = filename[i];
        if (c == '_') {
            if (j && out[j - 1] == '_') continue; // Don't allow consecutive underscores
            out[j++] = '_';
        } else if (c == ' ') {
            if (j && out[j - 1] == ' ') continue; // Don't allow consecutive spaces
            out[j++] = ' ';
        } else if ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
            out[j++] = c;
        }
    }
    if (j > 0 && out[j - 1] == '_') j--; // Don't end with an underscore
    out[j] = '\0';
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