# File syncing

There are two places in the system where files are synchronized between the file system and some remote storage.

1. For ingest, files are usually synced from Dropbox or whatever into a directory watched by `preprocessor-folder`
2. For `upload` files are synced one-by-one onto another storage platform like Google Drive

## Rclone configuration

In either case, you will need to configure `rclone` to support the "remote" you want to use. As long as you are familiar with the terminal this should be a pretty easy process.

1. First, make sure [rclone](https://rclone.org/) is installed on both the server and your local dev machine
2. Run `rclone config`, then follow the instructions. For example:

```
sysadmin@integrity-v2-demo:~$ rclone config
Current remotes:

Name                 Type
====                 ====
drive                drive

e) Edit existing remote
n) New remote
d) Delete remote
r) Rename remote
c) Copy remote
s) Set configuration password
q) Quit config
e/n/d/r/c/s/q> n

Enter name for new remote.
name> dropbox

Option Storage.
Type of storage to configure.
Choose a number from below, or type in your own value.
 1 / 1Fichier
   \ (fichier)
 2 / Akamai NetStorage
   \ (netstorage)

<snip>

12 / Dropbox
   \ (dropbox)

<snip>

56 / seafile
   \ (seafile)
Storage> dropbox

Option client_id.
OAuth Client Id.
Leave blank normally.
Enter a value. Press Enter to leave empty.
client_id>

Option client_secret.
OAuth Client Secret.
Leave blank normally.
Enter a value. Press Enter to leave empty.
client_secret>

Edit advanced config?
y) Yes
n) No (default)
y/n> n

Use web browser to automatically authenticate rclone with remote?
 * Say Y if the machine running rclone has a web browser you can use
 * Say N if running rclone on a (remote) machine without web browser access
If not sure try Y. If Y failed, try N.

y) Yes (default)
n) No
y/n> n

Option config_token.
For this to work, you will need rclone available on a machine that has
a web browser available.
For more help and alternate methods see: https://rclone.org/remote_setup/
Execute the following on the machine with the web browser (same rclone
version recommended):
        rclone authorize "dropbox"
Then paste the result.
Enter a value.
config_token>
```

At this point I run `rclone authorize "dropbox"` on my laptop as directed. It opens my browser and I log in to Dropbox. Now I see this in my laptop terminal (censored for security):

```
Paste the following into your remote machine --->
{"access_token":"...
<---End paste
```

I paste that back into my server terminal.

```
config_token> {"access_token":"...

Configuration complete.
Options:
- type: dropbox
- token: {"access_token":"...
Keep this "dropbox" remote?
y) Yes this is OK (default)
e) Edit this remote
d) Delete this remote
y/e/d>

Current remotes:

Name                 Type
====                 ====
drive                drive
dropbox              dropbox

e) Edit existing remote
n) New remote
d) Delete remote
r) Rename remote
c) Copy remote
s) Set configuration password
q) Quit config
e/n/d/r/c/s/q> q
```

Now rclone has access to Dropbox. I can test this out with `rclone lsd dropbox:`.

## File ingest

If you want to ingest files into the pipeline, there is already a CLI tool to help with that, `sync`.

```
$ starling sync
The sync command uses the provided arguments to run "rclone sync" in a loop.

All arguments are passed to "rclone sync", and then the command is executed in a loop,
with a 30 second delay between runs. The loop stops if the command fails.
```

To understand how to use the sync command, you can read the documentation on `rclone sync`, available [here](https://rclone.org/commands/rclone_sync/).

For example, to download files in a folder called `demo-2024` from a remote called `work_gdrive2` into an ingest folder at `~/integrity-sync/demo`:

```
starling sync work_gdrive2:/demo-2024 ~/integrity-sync/demo
```

The first time you try this, you might want to use `--interactive` or `--dry-run` to see what will happen.

Note that because this is a sync, files deleted on the remote will be deleted in the local folder as well. But they will not be un-ingested.

## Asset upload

Running `starling file upload` should provide enough help for this. Any remote added to rclone can be used with the upload tool. Example:

```
starling file upload work_gdrive2:/demo-2024-exports bafy1...
```
