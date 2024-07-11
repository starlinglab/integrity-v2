# Quickstart    
This Doc will walk you through getting quickly setup with the Starling Lab Integrity V2 CLI tool, and using some basic commands to interact with, upload, and add attributes to files.  

## What is Authenticated Attributes?
Authenticated Attributes (AA) is a software project from The Starling Lab. It uses an authenticated database, alongside modern cryptography to enable individuals and groups to securely store and share media and metadata. Using this system means the integrity and authenticity of files and metadata stored in the tool remain secure, even in the face of deepfakes and misinformation. It also enables data relationships, linking one piece of evidence to another, even across different organizations.

The Authenticated Attributes database consists of separate _'attributes'_. An asset (a media or any type of file) is one attribute, identified with a CID, and any metadata, in the form of a key:value pair that is added,  is related to the CID of the asset as another attribute. Each _attrbibute_ is signed and timestamped, and if this attribute is edited, it is appended as an edit to the original attribute. When digital media and metadata is added to our database, it which provides:
* Trusted timestamping with OpenTimestamps
* Cryptographic signatures of metadata
* An unchangeable record of edit history


## Installation
First you will need to install the correct binary for your computer, found here: [https://github.com/starlinglab/integrity-v2/releases](https://github.com/starlinglab/integrity-v2/releases)

First, `cd` into the directory that contains the binary and config files with the JWT and other settings

All local commands will be appended with `./starling` at the beginning, for example `./starling search cids` 

> Note - you may have to go into privacy and security settings (on mac) and allow this CLI appication to be run on your computer

### Config File
Alongside the binary, (wherever it's installed) you need to have a config file names `integrity-v2.toml` file, with at least the following. See the [example toml file](/example_config.toml) in this repo

```
[aa]
# Actual config for AA service is provided in .env file
# Any settings in this section are just for AA users
url = "http://localhost:3001"
jwt = "foo.bar.baz"           
```

> note, without a jwt you will not be able to do modification commands like `attr set`. The current url is `"https://aa.dev.starlinglab.org"`

## Search Assets & Indicies
All assets in Authenticated Attributes are given a Content IDentifier (CID). You can search for these CIDs and see the attributes associated with these assets

To view all the CID (Content IDentifiers) of the files use 
```
./starling attr search cids
```
#### You need the *CID* of an asset to see all it's properties or *attributes*

See all the attributes of a file
```
./starling attr get -all <CID>
```

Assets are organized into collections or *Projects*
You can see which projects exist/ are indexed with

```
./starling attr search index project_id 
```

To see the different assets in the project, search the index by `project_id `

```
./starling attr search index project_id ipfs-camp-demo
```

To search for a CID if you know the file name (all commands must use CID as the identifier) use:

```
./starling attr search index file_name "1715212515733.jpg"
```

#### There are 3 attributes that assets are indexed by. You can search by the attributes: `file_name`, `asset_origin_id`, or `project_id`


Search by the `asset_origin_id` to see a listing of paths. The path shows you which project, and type of content you have.
```
./starling attr search index asset_origin_id
...
/ipfs-camp-demo/proofmode-0x5b2c9a2821cea15a-2024-07-02-11-07-09gmt-05:00.zip/img_20240702_110659.jpg
```

</br>
</br>
</br>
-------------------------------------------------------
</br>
</br>
</br>

## Get Attributes
Attributes are individually signed and timestampes piece of metadata associated with an asset's CID.

**The different Attributes Include**
- Hashes (hex strings): `sha256`, `blake3`, `md5`
- File info (from file preprocessor) `file_name`, `file_size`, `last_modified`
- `time_created`: when the asset was originally created
- `description`: human description added manually
- `name`: human name added manually
- `asset_origin_id`: a human identifier for the file such as a file name or internal ID like `ABC-123`
- `asset_origin_type`: an array of strings that identify the kind of asset, like `folder`, `proofmode`, or `wacz`. 
- `author`: an organization or person according to the [schema](https://schema.org/author)

_See more in [attributes.md](/docs/attributes.md)_

With the CLI you install on your local machine, you can use the commands, appended with `attr`, including `get`, `set`, `search`, and `export`. Note that you cannot encrypt or inspect encrypted attributes.

Check what commands and modifiers you have available in each `attr` command, which consist of either `[get, set, search, export]`

If you type in 
```
./starling attr get
``` 
You can see all the commands you can append to `get`

### Get Examples 
Lets explore the individual attributes of one of the assets:

1. See when something was created 
```
./starling attr get --attr time_created <CID>
```

2. To find the file name of an asset with a given CID use: 
```
./starling attr get --cid <CID> --attr file_name
```

Looking at some examples, lets explore the output of a Proofmode file

1. First verify that a given CID is a Proofmode file
```
./starling attr get --attr asset_origin_type bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4
```

2. This was originally a proofmode file, however, there is an image and metadata, so if you look at what type of file is stored in Authenticated Attributes you see this CID is the identifier for ...
```
./starling attr get --attr media_type bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4 
```
..A jpeg image that was extracted from the proofmode bundle

Now lets look at a blockchain registration:
```
./starling attr get --attr registrations bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4 
```
You can see this is registered on the Near blockchain
