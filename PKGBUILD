pkgname="WiiUDownloader"
pkgver="1.11"
pkgrel="1"
pkgdesc="Allows to download encrypted wiiu files from nintendo's official servers"
arch=("x86_64")
depends=('mbedtls' 'gtkmm3' 'curl')
makedepends=('cmake')

build() {
    mkdir -p "../build"
    cd "../build"
    cmake ..
    make
}

package() {
    cd "../build"
    mkdir -p "$pkgdir/usr/local/bin"
    install WiiUDownloader -t "$pkgdir/usr/local/bin"
}