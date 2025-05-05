# Guide to Registering Assets Using the Integrity v2 CLI
The purpose of cryptographically registering assets is to establish immutable proof of a digital asset's existence, authenticity, and timestamp that cannot be forged or altered after the fact, creating verifiable provenance that helps combat fraud, misinformation, and AI-generated content Starlinglab. This cryptographic registration provides unique digital fingerprints that allow creators to maintain control and establish provenance for their work while enabling precise verification of content without exposing the original data. 

## Prerequisites

- You have the Starling CLI tool installed (`starling` binary)
- You have added your asset to the Authenticated Attributes database
- The asset has both `media_type` and `sha256` attributes (required for registration)

## Basic Registration Command
The basic format of the registration command is:

```
starling file register --on <blockchain> [options] <CID>
```

## Options
- `--include <attributes>`: Comma-separated list of additional attributes to include in registration
- `--testnet`: Register on a test network instead of mainnet
- `--dry-run`: Show what would be registered without actually sending it

## Examples

1. Register an asset on the Numbers Protocol blockchain:
   ```
   starling file register --on numbers bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
   ```

2. Include additional attributes in registration:
   ```
   starling file register --include media_type,author --on numbers bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
   ```

3. Test the registration process without actually registering:
   ```
   starling file register --on numbers --dry-run bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
   ```

4. Register on a test network:
Testnet registrations are not logged in the Authenticated Attributes database, making them consequence-free tests that won't affect your production environment – and wallet balance.
   ```
   starling file register --on numbers --testnet bafybeibqzv26nf3i5lzwjooqqoowe3krgynsaeamwu6sqrkjsumel7crsm
   ```

## Verification

After successful registration, the transaction details will be stored in the `registrations` attribute of your asset. You can view this with:

```
starling attr get --attr registrations <CID>
(...)
  "registrations": [
    {
      "attrs": [
        "media_type", "author"
      ],
      "chain": "numbers",
      "data": {
        "assetCid": "bafkrei...hcxoke",
        "assetTreeCid": "bafkrei...5ghhqi",
        "order_id": "882af7df...6ec7b3309",
        "txHash": "0x1a4058...41a5b97c052"
      }
    }
  ],
(...)
```


This will show details like the transaction hash, blockchain used, and which attributes were included in the registration.

## Notes

- Registration requires a valid configuration in your config file
- The Numbers Protocol requires a token to be set in your config file
