cmake_minimum_required(VERSION 3.10)
project(WiiUDownloader)

set(CMAKE_CXX_STANDARD 20)

list(APPEND CMAKE_MODULE_PATH ${CMAKE_SOURCE_DIR}/cmake)

find_package(PkgConfig)
find_package(MbedTLS)

if (PKG_CONFIG_FOUND)
  pkg_check_modules(GTKMM "gtkmm-3.0")
  if (GTKMM_FOUND)    
    link_libraries(${GTKMM_LIBRARIES})
    add_definitions(${GTKMM_CFLAGS} ${GTKMM_CFLAGS_OTHER})
  endif()
endif()
add_subdirectory(lib/nativefiledialog-extended)
execute_process(COMMAND uname OUTPUT_VARIABLE uname)
if(uname MATCHES "^MSYS" OR uname MATCHES "^MINGW")
  link_libraries(nfd ole32 uuid shell32)
  set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe -mwindows -mms-bitfields")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe -mwindows -mms-bitfields")
  file(GLOB SOURCES
          src/*.cpp
          src/*.c
          src/cdecrypt/*.c
          src/miniz/*.c
          )
else()
  link_libraries(nfd)
  set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe")
  file(GLOB SOURCES
          src/*.cpp
          src/*.c
          src/cdecrypt/*.c
          )
endif()

include(GResource)
gresource(${CMAKE_CURRENT_SOURCE_DIR}
            data/wiiudownloader.gresource.xml
            ${CMAKE_CURRENT_SOURCE_DIR}/src
            RESOURCE_FILE)

link_directories(
    ${GTKMM_LIBRARY_DIRS}
    /usr/local/lib
    /opt/homebrew/lib
)

include_directories(
    ${CMAKE_CURRENT_SOURCE_DIR}
    ${GTKMM_INCLUDE_DIRS}
    include
    /usr/local/include
    /opt/homebrew/include
)

add_executable(${CMAKE_PROJECT_NAME}
                ${SOURCES}
                ${RESOURCE_FILE})

if(uname MATCHES "^MSYS" OR uname MATCHES "^MINGW")
  add_subdirectory(lib/regFixLongPaths)
  add_dependencies(${CMAKE_PROJECT_NAME} regFixLongPaths)
endif()

target_link_libraries(${CMAKE_PROJECT_NAME}
    ${GTKMM_LIBRARIES}
    stdc++
    mbedtls
    mbedcrypto
    mbedx509
    curl
)

enable_testing()
add_subdirectory(lib/Catch2)
add_executable(tests
        src/titleInfo.c
        src/gtitles.c
        src/utils.cpp
        src/keygen.cpp
        src/cdecrypt/aes.c
        src/cdecrypt/cdecrypt.c
        src/cdecrypt/sha1.c
        src/cdecrypt/util.c
        src/downloader.c
        src/fst.c
        src/settings.cpp
        src/jsoncpp.cpp
        tests/tests.cpp)

target_link_libraries(tests
        stdc++
        mbedtls
        mbedcrypto
        mbedx509
        curl
        Catch2::Catch2WithMain)

include(CTest)
include(Catch)
catch_discover_tests(tests)