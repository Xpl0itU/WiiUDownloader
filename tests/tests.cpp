#include <catch2/catch_test_macros.hpp>
#include <downloader.h>
#include <keygen.h>
#include <utils.h>

#include <sstream>
#include <iomanip>

#define OSV0_TICKET_PATH "OSv0 [System App] [0005001010004000]/title.tik"
#define OSV0_TITLE_KEY_HASH "1b9e101f715a02e371c9b364904023689c807f35b85896faf5fbc999bef236df"

static char *uint8ArrayToCString(const uint8_t *data, size_t dataLen) {
    std::stringstream ss;
    for (int i = 0; i < dataLen; i++) {
        ss << std::hex << std::setw(2) << std::setfill('0') << (int) data[i];
    }

    std::string str = ss.str();
    char *c_str = new char[str.length() + 1];
    strcpy(c_str, str.c_str());

    return c_str;
}

TEST_CASE("Title testing", "[titles]") {
    setSelectedDir(".");
    bool cancelQueue = false;
    SECTION("Title downloads") {
        int downloadValue = downloadTitle("0005001010004000", "OSv0", false, false, false);
        REQUIRE(downloadValue == 0);
    }

    SECTION("Title resuming and decryption") {
        int downloadValue = downloadTitle("0005001010004000", "OSv0", true, false, false);
        REQUIRE(downloadValue == 0);
    }

    SECTION("Ticket TitleKey verification") {
        int hashValue = -6;
        if(fileExists(OSV0_TICKET_PATH)) {
            FILE *tik = fopen(OSV0_TICKET_PATH, "rb");
            if(tik != nullptr) {
                size_t fSize = getFilesizeFromFile(tik);
                if(fSize) {
                    uint8_t *buffer = (uint8_t *) malloc(fSize);
                    fread(buffer, fSize, 1, tik);
                    TICKET *ticket = (TICKET *) buffer;

                    char *titleKey = uint8ArrayToCString(ticket->key, 0x10);
                    hashValue = compareHash(titleKey, OSV0_TITLE_KEY_HASH);

                    delete[] titleKey;
                    free(buffer);
                    fclose(tik);
                }
            }
        }
        REQUIRE(hashValue == 0);
    }
}