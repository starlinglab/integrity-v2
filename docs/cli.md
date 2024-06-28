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
- Group: `file` (server-only)
  - `decrypt`: decrypt an encrypted file
  - `encrypt`: encrypt a file already stored in the system
  - `cid`: calculate a CIDv1 for a file
  - `c2pa`: inject a file with AA metadata using C2PA
  - `register`: register a file with a third-party blockchain
  - `upload`: upload a file to a third-party storage provider
- `genkey`: create a cryptographic key for use with Authenticated Attributes


Examples:
```
starling attr get
starling attr set
starling attr export
starling attr search
starling genkey
starling file upload
starling file encrypt
starling file decrypt
starling file register
starling file cid
starling file c2pa
```

## Demo

[![asciicast](https://asciinema.org/a/vFtmerhaiwa0P768d9dYPGTnM.svg)](https://asciinema.org/a/vFtmerhaiwa0P768d9dYPGTnM)
