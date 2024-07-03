# https://github.com/casey/just

set positional-arguments

prod := "0"

ldflags := if prod == "1" { "-s -w" } else { "" }

standalone:
    @mkdir -p build
    @go build -ldflags="{{ldflags}}" -o build/starling

build tool:
    @mkdir -p build
    @go build -ldflags="{{ldflags}}" -o build/$1 ./$1/cmd

# Remove binaries but not any custom made directories or whatever
clean:
    @find build -maxdepth 1 -type f -delete

releases:
    @mkdir -p build
    GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o build/starling_linux_amd64
    GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o build/starling_windows_amd64.exe
    GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o build/starling_mac_intel
    GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o build/starling_mac_apple
