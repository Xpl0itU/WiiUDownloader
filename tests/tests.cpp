#include <catch2/catch_test_macros.hpp>
#include <downloader.h>

TEST_CASE("Title downloads", "[download]") {
    setSelectedDir(".");
    bool cancelQueue = false;
    int downloadValue = downloadTitle("0005001010004000", "OSv0", false, &cancelQueue, false, false);
    REQUIRE(downloadValue == 0);
}

TEST_CASE("Title decryption", "[decryption]") {
    setSelectedDir(".");
    bool cancelQueue = false;
    int downloadValue = downloadTitle("0005001010004000", "OSv0", true, &cancelQueue, false, false);
    REQUIRE(downloadValue == 0);
}
