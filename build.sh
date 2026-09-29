#!/bin/bash

VERSION="${VERSION:-0.2.0}"
LDFLAGS="-s -w -X main.Version=${VERSION}"
BUILD_ALL=false

for arg in "$@"; do
    case $arg in
        --dist)
        BUILD_ALL=true
        shift
        ;;
    esac
done

if [ "$BUILD_ALL" = true ]; then

    go-winres make --arch amd64,arm64

    echo "Building Windows-AMD64..."

    GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/gguard.exe .

    zip -j dist/GopherGuard-Windows-x64.zip dist/gguard.exe
    rm -f dist/gguard.exe

    echo "Building Windows-ARM64..."

    GOOS=windows GOARCH=arm64 go build -ldflags "$LDFLAGS" -o dist/gguard.exe .

    zip -j dist/GopherGuard-Windows-ARM.zip dist/gguard.exe
    rm -f dist/gguard.exe

    echo "Building Linux-AMD64..."

    GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/gguard .

    tar -czf dist/GopherGuard-Linux-x64.tar.gz -C dist gguard
    rm -f dist/gguard

    echo "Building Linux-ARM64..."

    GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o dist/gguard .

    tar -czf dist/GopherGuard-Linux-ARM.tar.gz -C dist gguard
    rm -f dist/gguard

    unset GOOS
    unset GOARCH
else
    go-winres make &>/dev/null || true

    echo "Building..."

    go build -ldflags "$LDFLAGS" -o gguard .
fi

echo "Finished."