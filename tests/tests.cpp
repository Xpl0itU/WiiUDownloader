#include <catch2/catch_test_macros.hpp>
#include <downloader.h>
#include <keygen.h>
#include <utils.h>

#include <memory>
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
    auto c_str = std::make_unique<char[]>(str.length() + 1);
    strcpy(c_str.get(), str.c_str());

    return std::move(c_str).get();
}

TEST_CASE("Title testing", "[titles]") {
    setSelectedDir(".");
    bool cancelQueue = false;
    SECTION("Title downloads") {
        int downloadValue = downloadTitle("0005001010004000", "OSv0", false, cancelQueue, false, false);
        REQUIRE(downloadValue == 0);
    }

    SECTION("Title resuming and decryption") {
        int downloadValue = downloadTitle("0005001010004000", "OSv0", true, cancelQueue, false, false);
        REQUIRE(downloadValue == 0);
    }

    SECTION("Ticket TitleKey verification") {
        int hashValue = -6;
        if(fileExists(OSV0_TICKET_PATH)) {
            FILE *tik = fopen(OSV0_TICKET_PATH, "rb");
            if(tik != nullptr) {
                size_t fSize = getFilesizeFromFile(tik);
                if(fSize) {
                    auto buffer = std::make_unique<uint8_t>(fSize);
                    fread(buffer.get(), fSize, 1, tik);
                    TICKET *ticket = (TICKET *) buffer.get();

                    char *titleKey = uint8ArrayToCString(ticket->key, 0x10);
                    hashValue = compareHash(titleKey, OSV0_TITLE_KEY_HASH);

                    fclose(tik);
                }
            }
        }
        REQUIRE(hashValue == 0);
    }
}