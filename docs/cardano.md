# Cardano

integrity-v2 has preliminary support for registrations on the Cardano blockchain.
Once configured, you can use the `register` commmand to store metadata on the "preview"
testnet. For more information on registration in general, see other doc files like
[registrations.md](./registrations.md).

## Setup

- Create a directory to hold Cardano files, likely alongside other directories like `files` and `enc_keys`
- Install the `cardano-cli` binary from [GitHub](https://github.com/intersectmbo/cardano-node/releases)
- Sign up for [Blockfrost](https://blockfrost.io/) and get an API key for the Cardano preview chain

Now you can add to your config file:

```toml
[dirs]
# Other dirs...
cardano = "/path/to/cardano/storage/"

[bins]
# Other binary paths...
cardano_cli = "/usr/bin/cardano-cli"

[cardano]
blockfrost_api_key = "previewABC123"
```

## Registering

Now you can run the registration command:

```
starling file register --on cardano <CID>
```

The first time you try to register it will fail, because the newly created wallet
has no tokens. As directed by the command you can get free tokens from the
[faucet](https://docs.cardano.org/cardano-testnets/tools/faucet)
and try to register again, which will then succeed.

## Metadata

Currently metadata is registered by encoding it as JSON, chopping up that JSON string
into 64-byte strings, and then publishing it on Cardano as an array of strings. This
indirection is to get around various limitations on data that can be published to
the blockchain -- for example strings can only be 64 bytes long.

An example of what metadata looks like when published on the blockchain can be seen
[here](https://preview.cardanoscan.io/transaction/83d6d34c5f75faf0d441ffad3a537e4202325bb9eec3346b402907391df70985?tab=metadata).

