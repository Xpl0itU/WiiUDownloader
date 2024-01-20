#!/bin/bash

# In this configuration, the following dependent libraries are compiled:
#
# * zlib
# * c-ares
# * openSSL

# Compiler and path
PREFIX=$PWD/pkg/aria2go/aria2-lib
C_COMPILER="gcc"
CXX_COMPILER="g++"

NPROC=`nproc`
# check if nproc is a command
if ! [[ -x "$(command -v nproc)" ]]; then
    echo 'Error: nproc is not installed. Using sysctl for macOS' >&2
    $NPROC=`sysctl -n hw.logicalcpu` # macOS
fi

# Check tool for download
aria2c --help > /dev/null
if [[ "$?" -eq 0 ]]; then
    DOWNLOADER="aria2c --check-certificate=false"
else
    DOWNLOADER="wget -c"
fi

echo "Remove old libs..."
rm -rf ${PREFIX}
rm -rf _obj

## Version
ZLIB_V=1.2.11
OPENSSL_V=1.1.1w
C_ARES_V=1.24.0
ARIA2_V=1.37.0

## Dependencies
ZLIB=http://sourceforge.net/projects/libpng/files/zlib/${ZLIB_V}/zlib-${ZLIB_V}.tar.gz
OPENSSL=http://www.openssl.org/source/openssl-${OPENSSL_V}.tar.gz
C_ARES=http://c-ares.haxx.se/download/c-ares-${C_ARES_V}.tar.gz
ARIA2=https://github.com/aria2/aria2/releases/download/release-${ARIA2_V}/aria2-${ARIA2_V}.tar.bz2

## Config
BUILD_DIRECTORY=/tmp/

## Build
cd ${BUILD_DIRECTORY}

# zlib build
if ! [[ -e zlib-${ZLIB_V}.tar.gz ]]; then
    ${DOWNLOADER} ${ZLIB}
fi
tar zxvf zlib-${ZLIB_V}.tar.gz
cd zlib-${ZLIB_V}
PKG_CONFIG_PATH=${PREFIX}/lib/pkgconfig/ \
    LD_LIBRARY_PATH=${PREFIX}/lib/ CC="$C_COMPILER" CXX="$CXX_COMPILER" \
    ./configure --prefix=${PREFIX} --static
make -j`nproc`
make install
cd ..

# c-ares build
if ! [[ -e c&&res-${C_ARES_V}.tar.gz ]]; then
    ${DOWNLOADER} ${C_ARES}
fi
tar zxvf c-ares-${C_ARES_V}.tar.gz
cd c-ares-${C_ARES_V}
PKG_CONFIG_PATH=${PREFIX}/lib/pkgconfig/ \
    LD_LIBRARY_PATH=${PREFIX}/lib/ CC="$C_COMPILER" CXX="$CXX_COMPILER" \
    ./configure --prefix=${PREFIX} --enable-static --disable-shared
make -j`nproc`
make install
cd ..

# openssl build
if ! [[ -e openssl-${OPENSSL_V}.tar.gz ]]; then
    ${DOWNLOADER} ${OPENSSL}
fi
tar zxvf openssl-${OPENSSL_V}.tar.gz
cd openssl-${OPENSSL_V}
PKG_CONFIG_PATH=${PREFIX}/lib/pkgconfig/ \
    LD_LIBRARY_PATH=${PREFIX}/lib/ CC="$C_COMPILER" CXX="$CXX_COMPILER" \
    ./config --prefix=${PREFIX}
make -j`nproc`
make install_sw
cd ..

# build aria2 static library
if ! [[ -e aria2-${ARIA2_V}.tar.bz2 ]]; then
    ${DOWNLOADER} ${ARIA2}
fi
tar jxvf aria2-${ARIA2_V}.tar.bz2
cd aria2-${ARIA2_V}/
# set specific ldflags if macOS
if [[ "$OSTYPE" == "darwin"* ]]; then
    export LDFLAGS="-L${PREFIX}/lib -framework Security"
fi
PKG_CONFIG_PATH=${PREFIX}/lib/pkgconfig/ \
    LD_LIBRARY_PATH=${PREFIX}/lib/ \
    CC="$C_COMPILER" \
    CXX="$CXX_COMPILER" \
    ./configure \
    --prefix=${PREFIX} \
    --without-sqlite3 \
    --without-libxml2 \
    --without-libexpat \
    --without-libgcrypt \
    --without-libssh2 \
    --with-openssl \
    --without-appletls \
    --without-libnettle \
    --without-gnutls \
    --without-libgmp \
    --enable-libaria2 \
    --enable-shared=no \
    --enable-static=yes
make -j`nproc`
make install
cd ..

# cleaning
rm -rf zlib-${ZLIB_V}
rm -rf c-ares-${C_ARES_V}
rm -rf openssl-${OPENSSL_V}
rm -rf aria2-${ARIA2_V}
rm -rf ${PREFIX}/bin

# generate files for c
cd ${PREFIX}/../
go tool cgo libaria2.go

echo "Prepare finished!"