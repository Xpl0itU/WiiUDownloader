cmake_minimum_required(VERSION 3.10)
project(WiiUDownloader)

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

add_subdirectory(nativefiledialog-extended)
execute_process(COMMAND uname OUTPUT_VARIABLE uname)
if(uname MATCHES "^MSYS" OR uname MATCHES "^MINGW")
  link_libraries(nfd ole32 uuid shell32 zip)
  set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe -mwindows")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe -std=c++20 -mwindows")
else()
  link_libraries(nfd)
  set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe")
  set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -D_GNU_SOURCE -UNDEBUG -DAES_ROM_TABLES -fvisibility=hidden -Ofast -fno-strict-aliasing -pipe -std=c++20")
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

file(GLOB SOURCES
    src/*.cpp
    src/*.c
    src/cdecrypt/*.c
    src/miniz/*.c
)

add_executable(${CMAKE_PROJECT_NAME}
                ${SOURCES}
                ${RESOURCE_FILE})

target_link_libraries(${CMAKE_PROJECT_NAME}
    ${GTKMM_LIBRARIES}
    stdc++
    mbedtls
    mbedcrypto
    mbedx509
    curl
)
