param(
    [switch]$Dist
)

$env:VERSION = if ($env:VERSION) { $env:VERSION } else { "0.2.0" }
$ldflags = "-s -w -X main.Version=$env:VERSION"

if ($Dist) {
    go-winres make --arch amd64,arm64

    Write-Host "Building Windows-AMD64..."

    $env:GOOS="windows"; $env:GOARCH="amd64"; go build -ldflags $ldflags -o dist\gguard.exe .

    Compress-Archive -Path dist\gguard.exe -DestinationPath dist\GopherGuard-Windows-x64.zip -Force
    Remove-Item -Force dist\gguard.exe

    Write-Host "Building Windows-ARM64..."

    $env:GOOS="windows"; $env:GOARCH="arm64"; go build -ldflags $ldflags -o dist\gguard.exe .

    Compress-Archive -Path dist\gguard.exe -DestinationPath dist\GopherGuard-Windows-ARM.zip -Force
    Remove-Item -Force dist\gguard.exe

    Write-Host "Building Linux-AMD64..."

    $env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags $ldflags -o dist\gguard .

    tar -czf dist\GopherGuard-Linux-x64.tar.gz -C dist gguard
    Remove-Item -Force dist\gguard

    Write-Host "Building Linux-ARM64..."

    $env:GOOS="linux"; $env:GOARCH="arm64"; go build -ldflags $ldflags -o dist\gguard .

    tar -czf dist\GopherGuard-Linux-ARM.tar.gz -C dist gguard
    Remove-Item -Force dist\gguard

    Remove-Item Env:GOOS
    Remove-Item Env:GOARCH
} else {
    go-winres make

    Write-Host "Building..."

    go build -ldflags $ldflags -o gguard.exe .
}

Write-Host "Finished."