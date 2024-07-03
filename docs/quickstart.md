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
./starling attr search cids
```

To search for a CID if you know the file name (all commands must use CID as the identifier) use:
```
starling attr search index file_name <filename.extension>
```


To search for a certain CID and see one of the [attributes](https://github.com/starlinglab/integrity-v2/blob/main/docs/attributes.md), such as the time it was created, use 
```
attr get --cid <CID> --attr time_created
``` 

See all attributes of a file
```
starling attr get -all <CID>
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

You can see which projects exist/ are indexed with

```
attr search index project_id
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

To explore a file 


### Proofmode Files
When a proofmode 'bundle' (zip file) is uploaded to the Starling Integrity V2 Backend, the image is extracted from the bundle, and the other items, such as the .crt and .pubkey

If you search by
```
./starling attr search index asset_origin_id
...
/ipfs-camp-demo/proofmode-0x5b2c9a2821cea15a-2024-07-02-11-07-09gmt-05:00.zip/img_20240702_110659.jpg
```

The path shows you which project, and type of content you have.

## Exploring attributes of proofmode bundle

1. Get the CID of a file you know is a proofmode image
```
./starling attr search index file_name img_20240702_110659.jpg
```
2. Next, view the atttributes of a given piece of content
```
starling attr search attr <CID>
```
3. explore the values of specific attributes
```
starling attr get --attr <attribute> <CID>
```

4. OR explore all the values of all the attributes
```
starling attr get --all <CID>
```

> To see the contents of a proofmode bundle (which is encrypted) you will need to add the `--encrypted` flag to the command to the get command -- This must be done on the server

```
./starling attr get --attr proofmode --encrypted <CID>
```

_Since the key is on the server, we can only get this when we are on starling server_


## Adding C2PA Manifests
To add a manifest, we need to start with a template (on server at `integrity-data/c2pa_tmpls/ipfs_camp.json`). the global location for template is in `integrity-data/c2pa_tmpls/`
```
{
  "ta_url": "http://timestamp.digicert.com",
  "claim_generator": "Starling Lab Integrity v2",
  "assertions": [
    {
      "label": "starling.file",
      "data": {
		"file_name": "{{file_name}}"
      }
    }
  ]
}

```
The `{{file_name}}` will be replaced with the attribute "file_name" if it exists

*Preview the C2PA Manifest*
`starling file c2pa -dry-run --manifest ipfs_camp <CID>`

Now add the C2PA manifest
`starling file c2pa --manifest ipfs_camp`
> Output
>Injected file stored at /home/sysadmin/integrity-data/files/bafybeid7nmgu3ivrakzcygjl6mnycvfkzitqq2wyhdonkkh2kwe2itc6um
>Logged C2PA export and relationship to AuthAttr to the respective attributes: c2pa_exports, children

Take a look at the new CID created with `starling attr get --all <New CID>`
You should get something like
```
{
  "parents": {
    "derived": [
      "CID(bafybeiberuwy65k3t3dyny2zmnzqlbi5qyhhiucpbshzivhragc4zwqfji)"
    ]
  }
}
```
Retrieve the parent, and inspect the new `c2pa_exports` attribute `starling attr get --attr c2pa_exports bafybeiberuwy65k3t3dyny2zmnzqlbi5qyhhiucpbshzivhragc4zwqfji` you will get the c2pa manifest.

```
[
  {
    "cid": "CID(bafybeid7nmgu3ivrakzcygjl6mnycvfkzitqq2wyhdonkkh2kwe2itc6um)",
    "manifest": "ipfs_camp",
    "timestamp": "2024-07-03T14:17:57Z"
  }
]
```

## Registering Assets On-Chain 

## Exploring Expanded Capabilities with the Server

### Notes
*_Encryption_*

What gets encrypted? Just the Proofmode PGP key, and whatever user (on server only) sets:
`./starling attr set --attr description --str 'My secret description' --encrypted <CID>`

SSH into starling Server
ssh sysadmin@142.93.149.155
> note that you can run `starling` not `./starling`

Conventions
starling group (attr or  file) command - modifier <CID>