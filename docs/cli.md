# Command-line Interface

The main method for interacting with the Integrity v2 pipeline is through the CLI tools that have been developed. This document lists the basic functionality of all the CLI tools. Further information on using the tools can be discovered by using `--help` on the command.

## Usage

Depending on how the binary is built, each CLI tool can either be called by running `starling <group> <tool name>` or just `<tool name>`.

## CLI tool list
- Group: `attr` (remote/network)
  - `get`: get attributes in the Authenticated Attributes database
  - `set`: set attributes in the Authenticated Attributes database
  - `search`: search attributes, CIDs, and the index
  - `export`: export a single attestation as a file in various formats
  - `relate`: add relationships between CIDs
- Group: `file` (server-only)
  - `decrypt`: decrypt an encrypted file
  - `encrypt`: encrypt a file already stored in the system
  - `cid`: calculate a CIDv1 for a file
  - `c2pa`: inject a file with AA metadata using C2PA
  - `register`: register a file with a third-party blockchain
  - `upload`: upload a file to a third-party storage provider
- `genkey`: create a cryptographic key for use with Authenticated Attributes
- `sync`: run `rclone sync` in a loop, see [syncing.md](./syncing.md)

## Example workflow

```bash
$ starling attr search cids
bafkreiddlrl4tyb7qzarvswpy2ebtjh36gnlo7n353a5blomtw2b75hxfq
bafkreidogqfzz75tpkmjzjke425xqcrmpcib2p5tg44hnbirumdbpl5adu
bafybeia5e7jhfw6w5y6nct6oqit4mg4u5fsz45spjnvcbd2hac2egpxqyy
bafybeia66av636ldc55u2sojko3ix3hrvvu3fuqdt4neliypddlcarvklm
bafybeiahezkuza42lesruqy32wy3tro7ymbhdhb3mruem7ap6cqwddb2tu
bafybeiar2c5zm6pf3jtgqanvqr5trzhxap4ytjmie3gntg2ve5bozitymm
bafybeib3uecd2getqwdeubg477csmsqx54cucvijp7wk6zvt7j4ox34kjm
bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm

$ starling attr search attr bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
my_attr
uploads
media_type
registrations
test
test2
test3
author
sha256
children
c2pa_exports
time_created

$ starling attr get --attr media_type bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
image/png

$ starling attr get --all bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
# All the attributes at once
# (not shown here)

# Set attributes using starling attr set

# Some attributes are encrypted
$ starling attr get --attr test bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
error attestation is encrypted, use --encrypted or --key

# With --encrypted, the key will be found automatically and the attr will be decrypted
$ starling attr get --attr test --encrypted bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
hello

# Now let's work with the files, this requires being on the server

# Uploading to Google Drive and web3.storage

$ starling file upload drive:/my_folder bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
info: rclone remote 'drive' is of type 'drive'
Uploading 1 of 1...
Done.

$ starling file upload web3:cole-test bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
warning: the whole file will be loaded into memory by w3 for upload
Uploading 1 of 1...
Done.

# Register to blockchain
$ starling file register --include media_type --on numbers --testnet bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
Registering...

{"txHash":"0x94e23806f002dc6f34777977da3456a7cef52db61f0e810e521593f2de70cc5a","assetCid":"bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm","assetTreeCid":"baf
kreihigxi2j56fyn4kdooxkktunzcyfl2i5tw22lzmaii34ynzif377q","order_id":"38f6321f-2ff5-4b1e-9274-354fc064a754"}

Testnet registration not logged in AuthAttr

# Remove --testnet to register on the real blockchain, and log the action to AuthAttr

# Export attestation as Verifiable Credential

$ starling attr export --attr media_type --format vc -o my_test.vc.json bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm

# Now my_test.vc.json has a VC for the media_type attribute

# Finally, let's inject this PNG with C2PA information
# First, you must have a C2PA manifest template already made

$ cat c2pa_tmpls/demo.json
{
  "ta_url": "http://timestamp.digicert.com",

  "claim_generator": "TestApp",
  "assertions": [
    {
      "label": "stds.schema-org.CreativeWork",
      "data": {
        "@context": "https://schema.org",
        "@type": "CreativeWork",
        "author": ["{{author}}"]
      }
    }
  ],
  "credentials": ["{{time_created}}"]
}

# You can see, certain values are injected at runtime, like the author and the time_created
# Anything in "assertions" is injected directly, but "credentials" variables are injected as VCs

$ starling file c2pa --manifest demo bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
Injected file stored at /home/sysadmin/integrity-data/files/bafybeiccef5elff67736o7yx7msp4r3xrkuh3qtcdqp3dzofx3mh6k4ihm

# Now we can use this CID for other operations: upload, encrypt, register, etc.
```
