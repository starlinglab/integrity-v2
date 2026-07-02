# Cardano

integrity-v2 has preliminary support for registrations on the Cardano blockchain.
Once configured, you can use the `register` command to store metadata on either Cardano
mainnet or the "preview" testnet. The target network is selected by the `--testnet` flag:
without it, registration goes to **mainnet**; with `--testnet`, it goes to the **preview**
testnet. (Note: this means a bare `register --on cardano` now targets mainnet — earlier
versions always used preview.) For more information on registration in general, see other
doc files like [registrations.md](./registrations.md).

## Setup

- Create a directory to hold Cardano files, likely alongside other directories like `files` and `enc_keys`
- Install the `cardano-cli` binary from [GitHub](https://github.com/intersectmbo/cardano-node/releases)
- Sign up for [Blockfrost](https://blockfrost.io/) and get an API key for the network you
  intend to use. Blockfrost keys are network-scoped: a key for mainnet begins with `mainnet`,
  and a key for the preview testnet begins with `preview`. The key must match the network you
  register on, or registration fails fast with a "key does not match the selected network"
  error.

Now you can add to your config file (use a `mainnet…` key for mainnet, or a `preview…` key
for the preview testnet):

```toml
[dirs]
# Other dirs...
cardano = "/path/to/cardano/storage/"

[bins]
# Other binary paths...
cardano_cli = "/usr/bin/cardano-cli"

[cardano]
blockfrost_api_key = "mainnetABC123"
```

## Registering

To register on **mainnet**:

```
starling file register --on cardano <CID>
```

To register on the **preview** testnet instead, add `--testnet`:

```
starling file register --on cardano --testnet <CID>
```

The first time you try to register it will fail, because the newly created wallet
has no funds. On the preview testnet you can get free tokens from the
[faucet](https://docs.cardano.org/cardano-testnets/tools/faucet); on mainnet you must
send real ADA to the wallet address. Once funded, register again and it will succeed.

## Metadata

Currently metadata is registered by encoding it as JSON, chopping up that JSON string
into 64-byte strings, and then publishing it on Cardano as an array of strings. This
indirection is to get around various limitations on data that can be published to
the blockchain -- for example strings can only be 64 bytes long.

An example of what metadata looks like when published on the blockchain can be seen
[here](https://preview.cardanoscan.io/transaction/83d6d34c5f75faf0d441ffad3a537e4202325bb9eec3346b402907391df70985?tab=metadata).

## Fees

The transaction fee is calculated dynamically. The current protocol fee
parameters (`min_fee_a`, `min_fee_b`) are fetched from Blockfrost, the
transaction is built and signed once to measure its on-chain size, and the fee
is computed as `min_fee_b + min_fee_a × size` (plus a tiny safety margin). The
change returned to the wallet is the input amount minus this fee.

## Testing the chain integration

The Cardano registration path (`register/cardano.go`) talks to the chain via Blockfrost. The tests
that exercise it live in `register/cardano_integration_test.go` and are gated on environment
variables, so the normal `go test ./...` stays offline and skips them. Run them with
`go test -run 'Cardano|Blockfrost' ./register/` (or, with [`just`](https://github.com/casey/just)
installed, `just test-cardano`). Each test picks its network from the `BLOCKFROST_PROJECT_ID`
prefix — a `preview…` key targets the preview testnet, a `mainnet…` key targets mainnet — so the
same commands work for either network just by swapping the key.

`TestBlockfrostReadPath` is a free, wallet-free read-only check: it confirms the live API behaves as
the confirmation polling assumes (an unknown transaction returns HTTP 404, a confirmed one returns
`block_height` / `block_time`). On mainnet, supply a known hash with `CARDANO_CONFIRMED_TX=<hash>`,
otherwise that portion is skipped while the 404 check still runs.

```
BLOCKFROST_PROJECT_ID=previewXXXX go test -v -run Blockfrost ./register/
```

`TestCardanoRegisterE2E` runs the whole build → sign → submit → poll path and spends real funds, so
it is opt-in via `CARDANO_E2E=1` (and `CARDANO_MAINNET_E2E=1` as a second guard on mainnet, where it
spends real ADA). On the first run the wallet is generated and the test fails printing an address to
fund; fund it from the faucet (preview) or by sending ADA (mainnet), then re-run.

```
BLOCKFROST_PROJECT_ID=previewXXXX \
CARDANO_CLI=/usr/local/bin/cardano-cli \
CARDANO_DIR=/path/to/cardano/storage \
CARDANO_E2E=1 go test -v -run Cardano ./register/
```

