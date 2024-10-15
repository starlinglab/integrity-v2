# Chris

This is an importer for a specifically-structured spreadsheet.

Clone this repo, copy the below example env into a `.env` file, and then run `go run chris.go`.

```env
METADATA_CSV="/home/user/Downloads/sheet.csv"
CIDS_CSV="cids.csv"
ASSET_ORIGIN_ID="foo-bar"
JWT="foobar"
```

- `METADATA_CSV`: path to CSV export of spreadsheet
- `CIDS_CSV`: path to two-column CSV mapping filenames/IDs (no extension) to CIDv1
- `ASSET_ORIGIN_ID`: the ID of the asset in the spreadsheet to import
- `JWT`: the AA server JWT

Note only the single asset defined by `ASSET_ORIGIN_ID` is actually imported.
