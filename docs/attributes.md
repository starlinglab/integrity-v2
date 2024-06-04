# Database Attributes

For the Integrity v2 pipeline, we have some specific database keys, or asset attributes, that we have defined in advance to have a certain schema and meaning. This document lists them.

Note all timestamps are stored as strings in RFC 3339 format.

All data is natively stored in DAG-CBOR encoding.

## Basic asset/file metadata

- Hashes (hex strings): `sha256`, `blake3`, `md5`
- File info (from file preprocessor) `file_name`, `file_size`, `last_modified`
- `time_created`: when the asset was originally created
- `description`: human description added manually
- `name`: human name added manually

## Other

### `c2pa_exports`

An array of objects indicating how this asset has been exported/injected with C2PA in the past. Only three keys are allowed: `cid` (DAG-CBOR CID bytes), `manifest` (name of the c2patool manifest), `timestamp` (time of export).

Example:

```javascript
[
  {
    cid: CID(bafybeicr7num2b752ymjhocqu5i3642ek7r5m2og6y7rvjx4fk4miflyam),
    manifest: "demo",
    timestamp: "2024-05-22T14:37:23-04:00"
  },
  {
    cid: CID(bafybeifiy4b2gpocu3ojjiqxmdoog53ymkxieslmomiydjucteacguscpu),
    manifest: "demo2",
    timestamp: "2024-05-22T14:46:01-04:00"
  }
]
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
    timestamp: "2024-05-29T20:02:12Z"
  },
  {
    path: "/foo/bar",
    service_name: "my_drive_for_work",
    service_type: "drive",
    timestamp: "2024-05-29T20:02:54Z"
  },
  {
    path: "dev-test",
    service_name: "web3",
    service_type: "web3.storage",
    timestamp: "2024-05-29T20:04:47Z"
  }
]
```

### `registrations`

An array of objects indicating what networks/blockchains this asset has been registered with. Three keys are required.

Example:

```javascript
[
  {
    // Metadata attributes that were registered
    "attrs": ["test","test2"],
    "chain": "numbers", // Network/blockchain
    // Receipt / custom data returned by registration API
    "data": {
      "assetCid": "bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm",
      "assetTreeCid": "bafkreigeszf54jrgvltbqd5awzpx3zdgv47d7p6tdhja5k2z3ea3uyrdru",
      "order_id": "4dd7cd58-94f6-4aaa-8f24-b10bd41235d5",
      "txHash": "0x78d30a13e4e6d38f8e574d381392152c88b1d40d4804763e7f080d18f968d625"
    }
  },
  // Minimal example
  {
    "attrs": [],
    "chain": "my_chain",
    "data": {}
  }
]
```
