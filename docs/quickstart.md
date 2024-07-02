# Quickstart    
This Doc will walk you through getting quickly setup with the Starling Lab Integrity V2 CLI tool, and using some basic commands to interact with, upload, and add attributes to files.  

## What is Authenticated Attributes?
Authenticated Attributes (AA) is a software project from The Starling Lab. It uses an authenticated database, alongside modern cryptography to enable individuals and groups to securely store and share media and metadata. Using this system means the integrity and authenticity of files and metadata stored in the tool remain secure, even in the face of deepfakes and misinformation. It also enables data relationships, linking one piece of evidence to another, even across different organizations.

The Authenticated Attributes plugin enables us to add the AA database. When digital media and metadata is added to our database, it which provides:
* Trusted timestamping with OpenTimestamps
* Cryptographic signatures of metadata
* An unchangeable record of edit history


## Installation
First you will need to install the correct binary for your computer, which will be provided publicly later.

First, `cd` into the directory that contains the binary and config files with the JWT and other settings

All commands will be appended with `./integrity-v2` at the beginning, for example `./integrity-v2 search cids` 

> Note - you may have to go into privacy and security settings (on mac) and allow this appication to be run on your computer

### Config File
Alongside the binary, (wherever it's installed) you need to have a config file names `integrity-v2.toml` file, with at least the following. See the [example toml file](/example_config.toml) in this repo

```
[aa]
# Actual config for AA service is provided in .env file
# Any settings in this section are just for AA users
url = "http://localhost:3001"
jwt = "foo.bar.baz"           
```

> note, without a jwt you will not be able to do `attr set`. The current url is `"https://aa.dev.starlinglab.org"`

## Search Files and Attributes

To view all the CID (Content IDentifiers) of the files use 
```
search attr cids
```

To search for a certain CID and see one of the [attributes](https://github.com/starlinglab/integrity-v2/blob/main/docs/attributes.md), such as the time it was created, use 
```
attr get --cid <CID> --attr time_created
``` 
Basic attributes you can use:
- Hashes (hex strings): `sha256`, `blake3`, `md5`
- File info (from file preprocessor) `file_name`, `file_size`, `last_modified`
- `time_created`: when the asset was originally created
- `description`: human description added manually
- `name`: human name added manually
- `asset_origin_id`: a human identifier for the file such as a file name or internal ID like `ABC-123`
- `asset_origin_type`: an array of strings that identify the kind of asset, like `folder`, `proofmode`, or `wacz`. Usually the array has just one value.
- `author`


To find the file name of an asset with a given CID use 
```
attr get --cid <CID> --attr file_name
```

To see all the files in this project by the attributes (that are indexed) `file_name`, `asset_origin_id`, or `project_id`
```
attr search index file_name
```

To search all of the files in a given project, use the command:
```
attr get --cid <CID> --attr project_id <Project name>
```

> Example for IPFS camp demo

```
attr get --cid <CID> --attr project_id <Project name> ipfs-camp-demo
```

## Exploring Files

To explore a certain file that you saw listed from the index. You will get a CID as output
```
./starling attr search index file_name 'patanal_19.jpg'
```

Once you have the CID you can use the other commands to explore propoerties or 'attributes' of this 


### Proofmode Files
