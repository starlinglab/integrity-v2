# Initial setup

1. Install AuthAttr
2. Install binary and dependencies
3. Create directories
4. Create database
5. Create config file
6. Run services

## Install AuthAttr

Authenticated Attributes is the centerpiece of this pipeline. Clone it, copy the `example.env` into a real `.env` file with your own modifications, and run it as a systemd service or just in a tmux window.

https://github.com/starlinglab/authenticated-attributes

## Install binary and dependencies

After building `starling`, transfer it to your server with `scp` and move it to `/usr/local/bin`.

You may also want to install optional dependencies such as [rclone](https://rclone.org), [c2patool](https://github.com/contentauth/c2patool), and [w3](https://web3.storage/docs/w3cli/).

## Create directories

The data pipeline requires several directories to store files. Here is one example setup.

```
integrity-data/
├── c2pa_tmpls
│   └── ...
├── enc_keys
│   └── ...
└── files
    └── ...
```

You may also want to create a default directory for different projects to store to-be-ingested files into,  named for example `integrity-sync`.

## Create database

Use the [docker-compose.yml](/docker-compose.yml) file to run the database and manager interface, and see [adminer-ui.md](./adminer-ui.md) for details.

Configure the database as described in [folder-preprocessor.md](./folder-preprocessor.md). This will include creating a project folder.

## Create config file

Please see [`example_config.toml`](./example_config.toml) for an example config file to use. The default path for this config is `/etc/integrity-v2/config.toml`, but it can live anywhere.

Environment variables, for configuration outside of the config file:

- `INTEGRITY_CONFIG_PATH`: path to config file if not at the default path. Note any file named `integrity-v2.toml` in the current directory will automatically be checked as well if the default path doesn't exist.
- `JWT_SECRET`: set the JWT secret for web services like the webhook
- `TMPDIR`: set the directory for temporary files, `/var/tmp` by default

## Run services

Using either systemd services (not provided) or simple tmux windows, run the services you require. Usually this is `starling webhook` and `starling folder-preprocessor`. Note `starling webhook` requires the `JWT_SECRET` env var be set.

You may also want to run `starling sync` to auto-sync external files into an ingest folder.

## Public access

You may want to set up a reverse-proxy like Caddy or Nginx to make these localhost services like AuthAttr and the webhook available on the public internet with HTTPS.
