# https://github.com/casey/just

set positional-arguments

prod := "0"

ldflags := if prod == "1" { "-s -w" } else { "" }

standalone:
    @go build -ldflags="{{ldflags}}" -o build/integrity-v2

build tool:
    @rm -f build/$1
    @go build -ldflags="{{ldflags}}" -o build/$1 ./$1/cmd

clean:
    @rm -rf build/*
