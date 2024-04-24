# https://github.com/casey/just

set positional-arguments

standalone:
    @go build -o build/integrity-v2

build tool:
    @go build -o build/$1 ./$1/cmd
