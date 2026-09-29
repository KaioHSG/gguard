param(
    [switch]$Dist
)

if ($Dist) {
    go-winres make --arch amd64,arm64

    Write-Host "Building Windows-AMD64..."

    $env:GOOS="windows"; $env:GOARCH="amd64"; go build -o dist\gguard.exe .
    $env:GOOS="windows"; $env:GOARCH="amd64"; go build -ldflags "-H windowsgui" -o dist\gg-launcher.exe .\launcher\

    Compress-Archive -Path dist\gguard.exe,dist\gg-launcher.exe -DestinationPath dist\GopherGuard-Windows-x64.zip -Force
    Remove-Item -Force dist\gguard.exe,dist\gg-launcher.exe

    Write-Host "Building Windows-ARM64..."

    $env:GOOS="windows"; $env:GOARCH="arm64"; go build -o dist\gguard.exe .
    $env:GOOS="windows"; $env:GOARCH="arm64"; go build -ldflags "-H windowsgui" -o dist\gg-launcher.exe .\launcher\

    Compress-Archive -Path dist\gguard.exe,dist\gg-launcher.exe -DestinationPath dist\GopherGuard-Windows-ARM.zip -Force
    Remove-Item -Force dist\gguard.exe,dist\gg-launcher.exe

    Write-Host "Building Linux-AMD64..."

    $env:GOOS="linux"; $env:GOARCH="amd64"; go build -o dist\gguard .

    tar -czf dist\GopherGuard-Linux-x64.tar.gz -C dist gguard
    Remove-Item -Force dist\gguard

    Write-Host "Building Linux-ARM64..."

    $env:GOOS="linux"; $env:GOARCH="arm64"; go build -o dist\gguard .

    tar -czf dist\GopherGuard-Linux-ARM.tar.gz -C dist gguard
    Remove-Item -Force dist\gguard

    Remove-Item Env:GOOS
    Remove-Item Env:GOARCH
} else {
    go-winres make

    Write-Host "Building..."

    go build -o gguard.exe .
    go build -ldflags "-H windowsgui" -o gg-launcher.exe .\launcher\
}

Write-Host "Finished."