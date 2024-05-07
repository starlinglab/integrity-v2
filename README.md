# integrity-v2

The monorepo for Starling Lab's new integrity pipeline.

## Compilation

All software is written in Go, and uses [just](https://github.com/casey/just) for building. Only running on Linux is supported.

Run `just` or `just standalone` to build a standalone busybox-style binary that contains all the tools. Run `just build <toolname>` to build a specific tool, for example `just build dummy`. All binaries are stored inside the `build` directory.

## Contributing

See `main.go` and `dummy/` for an example of how to add a tool.

## License

This project is available under the MIT license. See [LICENSE](./LICENSE) for details.
