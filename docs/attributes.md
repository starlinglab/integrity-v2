# Database Attributes

For the Integrity v2 pipeline, we have some specific database keys, or asset attributes, that we have defined in advance to have a certain schema and meaning. This document lists them.

Note all timestamps are stored as strings in RFC 3339 format.

All data is natively stored in DAG-CBOR encoding.

## Table of Contents
- [Database Attributes](#database-attributes)
  - [Table of Contents](#table-of-contents)
  - [Basic asset/file metadata](#basic-assetfile-metadata)
  - [Ingest-specific](#ingest-specific)
    - [`proofmode`](#proofmode)
    - [`wacz`](#wacz)
  - [Process log attributes](#process-log-attributes)
    - [`c2pa_exports`](#c2pa_exports)
    - [`uploads`](#uploads)
    - [`registrations`](#registrations)


## Basic asset/file metadata

The majority of these attributes are set automatically upon ingestion, but all can be overrided manually later if needed.

- Hashes (hex strings): `sha256`, `blake3`, `md5`
- File info: `file_name`, `file_size`, `last_modified`
- `time_created`: when the asset was originally created
- `description`: human description added manually
- `name`: human name added manually
- `asset_origin_id`: a unique human-readable identifier for the file
  - Currently this is just the full file path
- `asset_origin_type`: an array of strings that identify the kind of asset, like `folder`, `proofmode`, or `wacz`. Usually the array has just one value.
- `author`: https://schema.org/author
- `project_id`: the name for the project this asset was ingested under
- `project_path`: the path for the project within the sync folder
- `asset_origin_sig_key_name`: may exist if the ingestion process involved verifiying a known, named public key
- Browsertrix crawl info: `crawl_workflow_name`, `crawl_workflow_description`, `crawl_workflow_tags`

Encrypted files have the `encryption_type` attribute, currently always set to `secretstream`. See [encryption.md](./encryption.md) for more info.

## Ingest-specific

There are specific, often cryptographic attributes added in different places, depending on the ingest process used. Looking at `asset_origin_type` will allow you to determine which of these attributes should exist.

### `proofmode`

When `asset_origin_type` contains `proofmode`, the `proofmode` attribute should exist. It contains the signature and timestamp information for the asset, as stored by Proofmode.

```json
{
  "metadata": "<string of proof.json>",
  "meta_sig": "<string of proof.json signature>",
  "media_sig": "<string of .asc>",
  "pubkey": "<string of pubkey.asc>",
  "ots": "<bytes of .ots>",
  "gst": "<string of .gst>"
}
```

### `wacz`

When `asset_origin_type` contains `wacz`, the `wacz` attribute should exist. It contains the `signedData` information for the WACZ file, extracted from within it (`datapackage-digest.json`).

Example data from Browsertrix cloud crawl:

```json
{
  "hash": "sha256:cf5f9bc8241dfb1b034512654b3605ba3110f1e04020d22a57b72818f1540394",
  "created": "2024-06-06T16:14:01Z",
  "software": "authsigner 0.5.0",
  "signature": "MEUCIH2uI8Ry6+j3PkIYjWL2YaFfwBgxZ25vPd5eL2KSfb1mAiEA8W5Ew2B0iR5AkMq5J52VKw9nvTUFvyfaz0/nc9ngJOM=",
  "domain": "signing.app.browsertrix.com",
  "domainCert": "-----BEGIN CERTIFICATE-----\nMIIEN<snip>",
  "timeSignature": "MIIFR<snip>",
  "timestampCert": "-----BEGIN CERTIFICATE-----\nMIIIA<snip>",
  "version": "0.1.0"
}
```

Example data from manual ArchiveWeb.page crawl:

```json
{
  "hash": "sha256:aec9680b63b6f04b9bf6c91ac2a31a030030288c0cc396e6f44611864b42622c",
  "signature": "93HpcVMOCebOGkESvfZZ5L13Dxi4Tsu306OOdHACduxFzpgHEvscyubERhPPchB0RZc+ICSqr25pmf7FQIMs2mUwiyhBR+diPCWHoi8OQD0ollEzQ8QiahM5r7ggQ4c9",
  "publicKey": "MHY<snip>",
  "created": "2024-04-17T17:02:07.848Z",
  "software": "Webrecorder ArchiveWeb.page 0.11.3, using warcio.js 2.2.0"
}
```

These fields are explained and defined in the [WACZ Signing and Verification](https://specs.webrecorder.net/wacz-auth/0.1.0/) spec.

## Process log attributes

These attributes serve to log various operations that were applied to the asset.

### `c2pa_exports`

An array of objects indicating how this asset has been exported/injected with C2PA in the past. Only three keys are allowed: `cid` (DAG-CBOR CID bytes), `manifest` (name of the c2patool manifest), `timestamp` (time of export).

Example:

```javascript
[
  {
    cid: CID(bafybeicr7num2b752ymjhocqu5i3642ek7r5m2og6y7rvjx4fk4miflyam),
    manifest: "demo",
    timestamp: "2024-05-22T14:37:23-04:00",
  },
  {
    cid: CID(bafybeifiy4b2gpocu3ojjiqxmdoog53ymkxieslmomiydjucteacguscpu),
    manifest: "demo2",
    timestamp: "2024-05-22T14:46:01-04:00",
  },
];
```

### `uploads`

An array of objects indicating what third-party storage providers this asset has been uploaded to in the past. Four keys are required, to show the service name, service type, upload path, and timestamp.

Example:

```javascript
[
  {
    path: "/foo",
    service_name: "drive",
    service_type: "drive",
    timestamp: "2024-05-29T20:02:12Z",
  },
  {
    path: "/foo/bar",
    service_name: "my_drive_for_work",
    service_type: "drive",
    timestamp: "2024-05-29T20:02:54Z",
  },
  {
    path: "dev-test",
    service_name: "web3",
    service_type: "web3.storage",
    timestamp: "2024-05-29T20:04:47Z",
  },
];
```

### `registrations`

An array of objects indicating what networks/blockchains this asset has been registered with. Three keys are required.

Example:

```javascript
[
  {
    // Metadata attributes that were registered
    attrs: ["test", "test2"],
    chain: "numbers", // Network/blockchain
    // Receipt / custom data returned by registration API
    data: {
      assetCid: "bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm",
      assetTreeCid: "bafkreigeszf54jrgvltbqd5awzpx3zdgv47d7p6tdhja5k2z3ea3uyrdru",
      order_id: "4dd7cd58-94f6-4aaa-8f24-b10bd41235d5",
      txHash: "0x78d30a13e4e6d38f8e574d381392152c88b1d40d4804763e7f080d18f968d625",
    },
  },
  // Minimal example
  {
    attrs: [],
    chain: "my_chain",
    data: {},
  },
];
```
