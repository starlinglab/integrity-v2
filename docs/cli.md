# Command-line Interface

The main method for interacting with the Integrity v2 pipeline is through the CLI tools that have been developed. This document lists the basic functionality of all the CLI tools. Further information on using the tools can be discovered by using `--help` on the command.

## Usage

Depending on how the binary is built, each CLI tool can either be called by running `starling <tool name>` or just `<tool name>`.

## CLI tool list

- `attr`: get and set attributes (attestations) in the Authenticated Attributes database
- `decrypt`: decrypt an encrypted file
- `encrypt`: encrypt a file already stored in the system
- `export-proof`: export a single attestation as a file in various formats
- `genkey`: create a cryptographic key for use with Authenticated Attributes
- `getcid`: calculate a CIDv1 for a file
- `inject-c2pa`: inject a file with AA metadata using C2PA
- `register`: register a file with a third-party blockchain
- `search`: search attestations, CIDs, and the index
- `upload`: upload a file to a third-party storage provider

## Demo

[![asciicast](https://asciinema.org/a/vFtmerhaiwa0P768d9dYPGTnM.svg)](https://asciinema.org/a/vFtmerhaiwa0P768d9dYPGTnM)
