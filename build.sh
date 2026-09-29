#!/bin/bash

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
    echo "Building Linux-AMD64..."
    GOOS=linux GOARCH=amd64 go build -o dist/gguard .
    tar -czf dist/GopherGuard-Linux-x64.tar.gz -C dist gguard
    rm -f dist/gguard

    echo "Building Linux-ARM64..."
    GOOS=linux GOARCH=arm64 go build -o dist/gguard .
    tar -czf dist/GopherGuard-Linux-ARM.tar.gz -C dist gguard
    rm -f dist/gguard

    go-winres make

    echo "Building Windows-AMD64..."
    GOOS=windows GOARCH=amd64 go build -o dist/gguard.exe .
    GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui" -o dist/gg-launcher.exe ./launcher/
    zip -j dist/GopherGuard-Windows-x64.zip dist/gguard.exe dist/gg-launcher.exe
    rm -f dist/gguard.exe dist/gg-launcher.exe

    echo "Building Windows-ARM64..."
    GOOS=windows GOARCH=arm64 go build -o dist/gguard.exe .
    GOOS=windows GOARCH=arm64 go build -ldflags "-H windowsgui" -o dist/gg-launcher.exe ./launcher/
    zip -j dist/GopherGuard-Windows-ARM.zip dist/gguard.exe dist/gg-launcher.exe
    rm -f dist/gguard.exe dist/gg-launcher.exe
else
    echo "Building..."
    go build -o gguard .
fi

echo "Finished."