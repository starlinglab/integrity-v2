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

All local commands will be appended with `./integrity-v2` at the beginning, for example `./integrity-v2 search cids` 

All commands on starling server are appended with `starling`

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

> note, without a jwt you will not be able to do `attr set`. The current url is `"https://aa.dev.starlinglab.org"`

## Search Assets and Attributes

### Asset Basics
To view all the CID (Content IDentifiers) of the files use 
```
./starling attr search cids
```

To search for a CID if you know the file name (all commands must use CID as the identifier) use:
```
starling attr search index file_name <filename.extension>
```

To see the different projects that assets are organizaed into, search by `project_id`
```
attr search index project_id <name-of-project>
```

> In general, to see all the files that are indexed, you can search by the attributes: `file_name`, `asset_origin_id`, or `project_id`



See all the attributes of a file
```
starling attr get -all <CID>
```


**Attributes**
- Hashes (hex strings): `sha256`, `blake3`, `md5`
- File info (from file preprocessor) `file_name`, `file_size`, `last_modified`
- `time_created`: when the asset was originally created
- `description`: human description added manually
- `name`: human name added manually
- `asset_origin_id`: a human identifier for the file such as a file name or internal ID like `ABC-123`
- `asset_origin_type`: an array of strings that identify the kind of asset, like `folder`, `proofmode`, or `wacz`. 
- `author`: an organization or person according to the [schema](https://schema.org/author)

_See more in [attributes.md](/docs/attributes.md)_

### Exploring Attributes
To search for a certain CID and see one of the [attributes](https://github.com/starlinglab/integrity-v2/blob/main/docs/attributes.md), such as the time it was created, use 
```
attr get --cid <CID> --attr time_created
``` 

To find the file name of an asset with a given CID use 
```
attr get --cid <CID> --attr file_name
```

You can see which projects exist/ are indexed with

```
attr search index project_id <project-name>
```

> Example for IPFS camp demo project

```
attr get --cid <CID> --attr project_id ipfs-camp-demo
```

## Example Workflow: Local CLI
With the CLI you install on your local machine, you can use the commands, appended with `attr`, including `get`, `set`, `search`, and `export`. Note that you cannot encrypt or inspect encrypted attributes.

_Unless you set it up differently, all commands will should be run in the folder where the binary is installed appended by `./starling`_

Check what commands and modifiers you have available in each command:
```
attr [get, set, search, export]
```

To explore a certain file that you saw listed from the index, search by  _You will get a CID as output_

```
attr search index file_name 'patanal_19.jpg'
```

Once you have the CID you can use the other commands to explore propoerties or 'attributes' of this piece of media

Search by the `asset_origin_id` to see a listing of paths. The path shows you which project, and type of content you have.
```
./starling attr search index asset_origin_id
...
/ipfs-camp-demo/proofmode-0x5b2c9a2821cea15a-2024-07-02-11-07-09gmt-05:00.zip/img_20240702_110659.jpg
```


## Example Workflow: Exploring expanded capabilities with the Starling server
If you establish an authenticated SSH connection within the Starling server, you have access to an expanded set of commands, appended with `file`, including `decrypt`, `encrypt`, `cid`, `c2pa`, `register`(on chain), `upload`, 

### Proofmode Files
When a proofmode 'bundle' (zip file) is uploaded to the Starling Integrity V2 Backend, the image is extracted from the bundle, and the other items, such as the .crt and .pubkey
Exploring attributes of proofmode bundle

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


### Adding C2PA Manifests
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
You will need tokens for this, currently the Starling Server. We use the Numbers API to register on numbers, avalanche, or near

Lets try a 'dry run' on the numbers testnet. use the flag `-on number` and `-testnet`
```
starling file register -dry-run -on numbers -testnet CID
```
_output_
>{
  "abstract": null,
  "assetCid": "bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4",
  "assetCreator": "Starling Lab",
  "assetSha256": "37867502fecf6e0ade0b9bfc8b539b34a9ed2b9aab952170a2def34345824ca2",
  "assetTimestampCreated": 1719918419,
  "custom": {
    "": null
  },
  "encodingFormat": "image/jpeg",
  "headline": null,
  "testnet": true
}

Now we can register on the testnet for real
```
starling file register -on numbers <CID>
```
_output example_
> `{"txHash":"0x142db493c4cb704106a8d5a3d29409f8f144f26d2c4aa23182ce8a48ea8a8943","assetCid":"bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4","assetTreeCid":"bafkreigzzqberl452vdmontyorchtd2eeiepkhf6b7tvya2asjsqdbeidq","order_id":"0c35413b-e178-41e9-9e0f-844ac68a0d19"}`

We can go to the near testnet block explorer, and search by `"txHash"`

> Note that testnet registrations aren't logged in Authenticated Attributes

* `"txHash"` is the transaction on numbers, you can explore it with [https://testnet.nearblocks.io/](https://testnet.nearblocks.io/) (testnet) or mainnet [https://nearblocks.io/](https://nearblocks.io/)
* `"Assetcid"` is the same CID that we had generated
* `"assetTreeCid"` is the manifest saved on IPFS. Use the gateway `https://ipfs-pin.numbersprotocol.io/ipfs` + the CID from `" the output. [Example](https://ipfs-pin.numbersprotocol.io/ipfs/bafkreigzzqberl452vdmontyorchtd2eeiepkhf6b7tvya2asjsqdbeidq) 

Now we can search and see the new attrbute

```
starling attr get --attr registrations  bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4
```

```
[
  {
    "attrs": [
      ""
    ],
    "chain": "near",
    "data": {
      "assetCid": "bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4",
      "assetTreeCid": "bafkreigzzqberl452vdmontyorchtd2eeiepkhf6b7tvya2asjsqdbeidq",
      "order_id": "2abd48f7-6369-442d-8144-f24f36aa7c10",
      "txHash": "0xd7bb74b5c63c799ec5583a7144839fac78d95b86dae2e5589fa3794e77a7cae5"
    }
  }
]
```


### Notes
*_Encryption_*

What gets encrypted? Just the Proofmode PGP key, and whatever user (on server only) sepcifies should be encrypted.

_Example_: Set the description attribute as encrypted for an asset with a given `<CID>` 
```
./starling attr set --attr description --str 'My secret description' --encrypted <CID>
```

SSH into starling Server
```
ssh sysadmin@142.93.149.155
```
> note that you append commands with `starling` not like in local CLI `./starling`

Conventions
starling <group> (attr or  file) command - modifier <CID>

#### CLI tool list
- Group: `attr` (remote/network)
  - `get`: get attributes in the Authenticated Attributes database
  - `set`: set attributes in the Authenticated Attributes database
  - `search`: search attributes, CIDs, and the index
  - `export`: export a single attestation as a file in various formats
- Group: `file` (server-only)
  - `decrypt`: decrypt an encrypted file
  - `encrypt`: encrypt a file already stored in the system
  - `cid`: calculate a CIDv1 for a file
  - `c2pa`: inject a file with AA metadata using C2PA
  - `register`: register a file with a third-party blockchain
  - `upload`: upload a file to a third-party storage provider
- `genkey`: create a cryptographic key for use with Authenticated Attributes

To see all off the commands & modifiers use `starling attr set` or `starling file c2pa`