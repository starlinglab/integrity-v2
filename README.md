# integrity-v2

The monorepo for Starling Lab's new integrity pipeline.

⚠️ STILL IN DEVELOPMENT ⚠️

## Compilation

All software is written in Go, and uses [just](https://github.com/casey/just) for building. Only running on Linux is supported. Go versions older than 1.22 are not supported.

Run `just` or `just standalone` to build a standalone busybox-style binary that contains all the tools. Run `just build <toolname>` to build a specific tool, for example `just build dummy`. All binaries are stored inside the `build` directory.

## Running

Please see [`example_config.toml`](./example_config.toml) for an example config file to use. The default path for this config is `/etc/integrity-v2/config.toml`, but it can live anywhere.

Environment variables, for configuration outside of the config file:

- `INTEGRITY_CONFIG_PATH`: path to config file if not at the default path. Note any file named `integrity-v2.toml` in the current directory will automatically be checked as well if the default path doesn't exist.
- `JWT_SECRET`: set the JWT secret for web services like the webhook
- `TMPDIR`: set the directory for temporary files, `/var/tmp` by default

Further instructions on running the software will be added in the future.

## Contributing

See `main.go` and `dummy/` for an example of how to add a CLI tool.

## License

This project is available under the MIT license. See [LICENSE](./LICENSE) for details.
