
## Description
This is a demo prepared for a presentation where people upload files synced to Authenticated Attributes, where files are ingested onto the server. The presenter should SSH into the server and run these commands from there

```
ssh sysadmin@142.93.149.155
```

# Hands-On Demo - IPFS Camp Uploads 
This piece is a walk-through for a demo of previewing assets that people have created using proofmode and uploaded to the Starling Integrity Backed using a GDrive folder
First, list the CIDs in the project that people have uploaded, then lets preview all assets available
```
./starling attr search index project_id ipfs-camp-demo
```

```
./starling attr get -all <CID>
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
starling attr get --attr registrations bafybeifbqqwj7625r2snojcksumwgcd3rmrdbvjo2rxiketsdefzbk7ia4
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

