# https://github.com/casey/just

set positional-arguments

standalone:
    @go build -o build/integrity-v2

build tool:
    @rm -f build/$1
    @go build -o build/$1 ./$1/cmd
