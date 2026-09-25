$env:Path = "C:\Program Files\Go\bin;$env:Path;$env:USERPROFILE\go\bin"

go-winres make
go build -o gguard.exe .
go build -ldflags "-H windowsgui" -o gg-launcher.exe .\launcher\