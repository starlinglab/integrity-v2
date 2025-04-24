# Quickstart
This document will walk you through getting quickly setup with Starling Lab's Integrity V2 CLI tool, and using some basic commands to interact with _Authenticated Attributes_ of files indexed by _Content IDentifiers (CIDs)_.

## What is Authenticated Attributes?
_Authenticated Attributes (AuthAttr)_ is a software project from The Starling Lab. It uses an authenticated database, alongside modern cryptography to enable individuals and groups to securely store and share media and metadata. Using this system means the integrity and authenticity of files and metadata stored in the tool remain secure, even in the face of deepfakes and misinformation. It also enables sharing of data relationships, linking of one piece of evidence to another, even across different organizations.

An asset (a media or any type of file) is identified with a CID, and an attribute is metadata in the form of a key-value pair that is associated with the CID of the asset. Each attribute is signed and timestamped, and if this attribute is edited, it is appended as an edit to the original attribute, allowing us to track them at a particular point in time. This is accomplished with:

* Author signature of each CID-key-value attribute
* Trusted timestamp with [OpenTimestamps](https://opentimestamps.org)
* Signed and subscriber-replicable changelog

## Installation
First you will need to install the correct binary for your computer, found here: [https://github.com/starlinglab/integrity-v2/releases](https://github.com/starlinglab/integrity-v2/releases).

Rename the downloaded binary to `starling` and make it executable: `$ chmod +x starling`.

❗️ On macOS, you have to go into Privacy & Security settings and allow this CLI application to be run on your computer.

### Config File
In the same directory as the binary, you need to have a config file named `integrity-v2.toml`. See the [example toml file](/example_config.toml) in this repo:

```
[aa]
url = "http://localhost:3001"
jwt = "foo.bar.baz"           
```

> Note - without a `jwt` you will not be able to do modification commands like `attr set`.

## Search Assets & Indicies
All assets in AuthAttr are given a CID. You can search for these CIDs and see the attributes associated with these assets.
There are 3 attributes that assets are indexed by. You can search by the attributes: `file_name`, `asset_origin_id`, or `project_id`

To view CIDs of all the files in the database:

```
./starling attr search cids
```

#### You need the CID of an asset to see all its attributes

See all the attributes of a file:

```
./starling attr get -all <CID>
```

Assets are organized into collections or *projects*. You can see which projects have indexed assets.

```
./starling attr search index project_id
```

To see the different assets in the project, search the index by `project_id`:

```
./starling attr search index project_id ipfs-camp-demo
```

To search for a CID if you know the file name:

```
./starling attr search index file_name "1715212515733.jpg"
```

You can also search `asset_origin_id` which contains information about the asset's origin when indexed:

```
./starling attr search index asset_origin_id
...
/ipfs-camp-demo/proofmode-0x5b2c9a2821cea15a-2024-07-02-11-07-09gmt-05:00.zip/img_20240702_110659.jpg
```

## Get Attributes
Attributes are individually signed and timestamped metadata associated with an asset's CID.

Some default attributes include:
- Hashes: `sha256`, `blake3`, `md5`
- File info (from file preprocessor): `file_name`, `file_size`, `last_modified`
- `name`: human-readable name added manually
- `description`: human-readable description added manually
- `author`: the creator organization or person according to the [schema](https://schema.org/author)
- `time_created`: when the asset was originally created
- `asset_origin_id`: an identifier for the file such as a file name or internal ID like `ABC-123`
- `asset_origin_type`: an array of strings that identify the kind of asset, like `folder`, `proofmode`, or `wacz`

See more in [attributes.md](/docs/attributes.md).

With the CLI you install on your local machine, you can use the commands, appended with `attr`, including `get`, `set`, `search`, and `export`. Note that you cannot encrypt or inspect encrypted attributes.

Check what commands and modifiers you have available in each `attr` command, which consist of either `[get, set, search, export]`

If you type in:

```
./starling attr get
``` 

You can see all the subcommands you can append to `get`.

### Get Examples 

**Lets explore the individual attributes of one of the assets.**

1. See when something was created:
```
./starling attr get --attr time_created <CID>
```

2. To find the file name of an asset with a given CID use: 
```
./starling attr get --cid <CID> --attr file_name
```

**Looking at some examples, lets explore the output of a ProofMode file:**

1. First verify that a given CID is a Proofmode file:

```
./starling attr get --attr asset_origin_type bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4
```

2. This was originally a ProofMode file, however, there is an image and metadata, so if you look at what type of file is stored in AuthAttr, you see this CID is the identifier for ...

```
./starling attr get --attr media_type bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4 
```

... a JPEG image that was extracted from the ProofMode bundle.

Now lets look at a blockchain registration:

```
./starling attr get --attr registrations bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4 
```

You can see this is registered on the Near blockchain.
