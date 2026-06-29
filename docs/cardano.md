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

The transaction fee is calculated dynamically rather than using a fixed overestimate. The
current protocol fee parameters (`min_fee_a`, `min_fee_b`) are fetched from Blockfrost, the
transaction is built and signed once to measure its on-chain size, and the fee is computed as
`min_fee_b + min_fee_a × size` (plus a tiny safety margin). The change returned to the wallet
is the input amount minus this fee, which must also clear the minimum UTXO floor described below.

## UTXO selection

To pay for a registration the wallet must spend one or more of its unspent transaction outputs
(UTXOs). Selection prefers **pure-ADA UTXOs, largest first**: it spends the largest plain-lovelace
output that covers the fee and the min-ada floor, keeping transactions to a single input where
possible. Only when the pure-ADA balance is insufficient does it pull in UTXOs that also carry
native tokens (again largest first). Whenever a spent UTXO carries native assets, those exact
assets are returned in the change output, so the transaction always preserves value and never burns
tokens. Selection targets `minAda + fee` (not just the fee) and automatically pulls in more UTXOs
until the change output clears the min-ada floor; it directs you to the faucet only when the whole
wallet still cannot produce a valid change output.

## Minimum UTXO (min-ada)

The ledger rejects any transaction output below a protocol **min-ada** floor (`OutputTooSmallUTxO`),
and that floor is higher for a token-bearing output because it depends on the output's serialized
size. Since a registration returns all value to a single change output back to the wallet, that
output must clear the floor.

We fetch `coins_per_utxo_size` from Blockfrost (a string field, e.g. `"4310"`) and compute the floor
as `(160 + estimatedTxOutBytes) × coins_per_utxo_size`, where `estimatedTxOutBytes` is a deliberately
**conservative over-estimate** of the output's byte size (covering the address, the lovelace amount,
and each native policy/asset). Over-estimating means the computed min-ada is always at least the
ledger's true value, so a change output funded to it never trips `OutputTooSmallUTxO`; the small
overpay simply returns to the wallet as change.

## Testing the chain integration

The Cardano registration path (`register/cardano.go`) talks to the chain via Blockfrost,
and that integration is validated independently of the Authenticated Attributes side —
`cardanoRegister` takes a plain message string and never touches AuthAttr, so these tests
use synthetic claims. They live in `register/cardano_integration_test.go` and are gated on
environment variables, so the normal `go test ./...` skips them and stays offline.

Run them with `go test -run 'Cardano|Blockfrost' ./register/` (or, if you have
[`just`](https://github.com/casey/just) installed, the shorthand `just test-cardano`).

The tests pick their network from the `BLOCKFROST_PROJECT_ID` prefix — a `preview…` key runs
them against the preview testnet, a `mainnet…` key against mainnet — so the same commands work
for either network just by swapping the key.

### Read-path check (free, no wallet)

`TestBlockfrostReadPath` needs only a Blockfrost `project_id`. Sign up at
[blockfrost.io](https://blockfrost.io), create a project on the network you want to test, and
copy the key. It makes read-only calls to the live API to confirm the assumptions the
confirmation polling depends on: that an unknown transaction returns HTTP 404 (the "still
pending" signal) and that a confirmed transaction returns `block_height` / `block_time`.

```
BLOCKFROST_PROJECT_ID=previewXXXX go test -v -run Blockfrost ./register/
```

The confirmed-transaction assertion uses a pinned preview transaction; on mainnet (which has no
pinned hash) supply one with `CARDANO_CONFIRMED_TX=<hash>`, otherwise just that portion is
skipped while the 404 check still runs.

### Full submit + confirm (spends funds)

`TestCardanoRegisterE2E` runs the whole build → sign → submit → poll path against the key's
network. It is opt-in via `CARDANO_E2E=1` and additionally needs `cardano-cli` and a directory
for the wallet:

```
BLOCKFROST_PROJECT_ID=previewXXXX \
CARDANO_CLI=/usr/local/bin/cardano-cli \
CARDANO_DIR=/path/to/cardano/storage \
CARDANO_E2E=1 go test -v -run Cardano ./register/
```

With a **mainnet** key this test spends real ADA, so it requires a second explicit opt-in,
`CARDANO_MAINNET_E2E=1`; without it the test skips loudly so a real-money transaction is never
broadcast by accident. (A preview key needs only `CARDANO_E2E=1`.)

On the first run the wallet is generated and the test fails, printing an address to fund — from
the [faucet](https://docs.cardano.org/cardano-testnets/tools/faucet) on preview, or by sending
real ADA on mainnet. Fund it, then re-run; the test asserts the returned record carries a
positive block height/time and `status: "confirmed"`, and logs how long confirmation actually
took (a sanity check on the polling timeout).

