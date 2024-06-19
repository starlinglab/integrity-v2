# https://github.com/casey/just

set positional-arguments

prod := "0"

ldflags := if prod == "1" { "-s -w" } else { "" }

standalone:
    @mkdir -p build
    @go build -ldflags="{{ldflags}}" -o build/integrity-v2

build tool:
    @mkdir -p build
    @go build -ldflags="{{ldflags}}" -o build/$1 ./$1/cmd

# Remove binaries but not any custom made directories or whatever
clean:
    @find build -maxdepth 1 -type f -delete
