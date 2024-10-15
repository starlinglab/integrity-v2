package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/starlinglab/integrity-v2/aa"

	_ "github.com/joho/godotenv/autoload"
)

var (
	metaCsv       = os.Getenv("METADATA_CSV")
	cidsCsv       = os.Getenv("CIDS_CSV")
	assetOriginId = os.Getenv("ASSET_ORIGIN_ID")

	knownHeader = []string{"asset_description", "Ingest?", "asset_origin_id", "asset_collection", "asset_event", "asset_subcollection", "asset_act", "asset_subject", "asset_sequence", "SIGNER:asset_*", "relationship:type", "relationship:asset", "sequence_relationship", "SIGNER:relationships", "produced_by:type", "produced_by:name", "produced_by:url", "SIGNER:produced_by", "original_url", "SIGNER:original_url", "asset_medium", "SIGNER:asset_medium", "capture_location", "SIGNER:capture_location", "caption", "SIGNER:caption", "capture_date", "SIGNER:capture_date", "camera", "SIGNER:camera", "lens", "SIGNER:lens", "camera_settings", "SIGNER:camera_settings"}

	chrisAA = &aa.AuthAttrInstance{
		Url:  "https://chris.aa.prod.starlinglab.org",
		Jwt:  os.Getenv("JWT"),
		Mock: false,
	}
	kiraAA = &aa.AuthAttrInstance{
		Url:  "https://kira.aa.prod.starlinglab.org",
		Jwt:  os.Getenv("JWT"),
		Mock: false,
	}
)

// https://schema.org/author
type author struct {
	Type string `json:"@type"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

func main() {
	// Read filename -> CID mapping first

	f, err := os.Open(cidsCsv)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	cidMap := make(map[string]string)
	r := csv.NewReader(f)
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		cidMap[record[0]] = record[1]
	}

	// Now read metadata

	headerCols := make(map[string]int, len(knownHeader))
	for i, s := range knownHeader {
		headerCols[s] = i
	}

	getCell := func(columnName string, row []string) string {
		i, ok := headerCols[columnName]
		if !ok {
			panic("unknown column " + columnName)
		}
		return row[i]
	}

	f, err = os.Open(metaCsv)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	r = csv.NewReader(f)

	header, err := r.Read()
	if err != nil {
		panic(err)
	}

	if !slices.Equal(header, knownHeader) {
		fmt.Printf("%#v\n", header)
		panic("header different than expected")
	}

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		if getCell("Ingest?", record) != "TRUE" {
			continue
		}

		aoi := getCell("asset_origin_id", record)
		if aoi != assetOriginId {
			continue
		}

		fmt.Println(aoi)

		cid := cidMap[aoi]
		if cid == "" {
			panic("no CID found in CIDs CSV")
		}

		// Confirm file has already been ingested
		if !chrisAA.Mock {
			_, err := chrisAA.GetAttestation(cid, "file_name", aa.GetAttOpts{})
			if err != nil {
				panic(err)
			}
		}

		// Collect key-value pairs for each signer

		kiraKVs := make([]aa.PostKV, 0)
		chrisKVs := make([]aa.PostKV, 0)

		// Start with asset_* fields, all one signer

		var kvs *[]aa.PostKV
		switch getCell("SIGNER:asset_*", record) {
		case "kira":
			kvs = &kiraKVs
		case "chris":
			kvs = &chrisKVs
		default:
			panic("unknown signer")
		}

		// Unindexed columns
		for _, s := range []string{
			"asset_description",
		} {
			v := getCell(s, record)
			if strings.TrimSpace(v) == "" {
				continue
			}
			*kvs = append(*kvs, aa.PostKV{Key: s, Value: v})
		}

		// Indexed columns
		for _, s := range []string{
			"asset_collection", "asset_subcollection", "asset_subject",
			"asset_sequence", "asset_act", "asset_event", "asset_origin_id",
		} {
			v := getCell(s, record)
			if strings.TrimSpace(v) == "" {
				continue
			}
			*kvs = append(*kvs, aa.PostKV{Key: s, Value: v, Type: "str"})
		}

		// Sequence relationship (indexed)

		switch getCell("SIGNER:relationships", record) {
		case "kira":
			kvs = &kiraKVs
		case "chris":
			kvs = &chrisKVs
		default:
			panic("unknown signer")
		}

		v := getCell("sequence_relationship", record)
		if strings.Contains(v, ",") {
			panic(v)
		}
		if strings.TrimSpace(v) != "" {
			*kvs = append(*kvs, aa.PostKV{
				Key:   "sequence_relationship",
				Value: v,
				Type:  "str",
			})
		}

		// produced_by author

		switch getCell("SIGNER:produced_by", record) {
		case "kira":
			kvs = &kiraKVs
		case "chris":
			kvs = &chrisKVs
		default:
			panic("unknown signer")
		}
		*kvs = append(*kvs, aa.PostKV{
			Key: "produced_by",
			Value: author{
				Type: getCell("produced_by:type", record),
				Name: getCell("produced_by:name", record),
				URL:  getCell("produced_by:url", record),
			},
		})

		// Simple text fields, each with their own signer
		for _, s := range []string{"original_url", "asset_medium", "capture_location",
			"caption", "camera", "lens", "camera_settings"} {
			v := getCell(s, record)
			if strings.TrimSpace(v) == "" {
				continue
			}
			switch getCell("SIGNER:"+s, record) {
			case "kira":
				kvs = &kiraKVs
			case "chris":
				kvs = &chrisKVs
			default:
				fmt.Println(s)
				panic("unknown signer")
			}
			*kvs = append(*kvs, aa.PostKV{Key: s, Value: v})
		}

		// Dates

		switch getCell("SIGNER:capture_date", record) {
		case "kira":
			kvs = &kiraKVs
		case "chris":
			kvs = &chrisKVs
		default:
			panic("unknown signer")
		}

		// Parse MM/DD/YYYY (Google Sheets) and store as YYYY-MM-DD (RFC 3339)
		var m, d, y int
		_, err = fmt.Sscanf(getCell("capture_date", record), "%d/%d/%d", &m, &d, &y)
		if err != nil {
			panic(err)
		}
		*kvs = append(*kvs, aa.PostKV{
			Key:   "capture_date",
			Value: fmt.Sprintf("%d-%02d-%02d", y, m, d),
		})

		// Send all the atts
		err = kiraAA.SetAttestations(cid, true, kiraKVs)
		if err != nil {
			panic(err)
		}
		err = chrisAA.SetAttestations(cid, true, chrisKVs)
		if err != nil {
			panic(err)
		}

		// Relationships

		var aaInstance *aa.AuthAttrInstance
		switch getCell("SIGNER:relationships", record) {
		case "kira":
			aaInstance = kiraAA
		case "chris":
			aaInstance = chrisAA
		default:
			panic("unknown signer")
		}

		relAssets := getCell("relationship:asset", record)
		if strings.TrimSpace(relAssets) != "" {
			for _, asset := range strings.Split(relAssets, ",") {
				relCid := cidMap[strings.TrimSpace(asset)]
				if relCid == "" {
					panic("relationship: no CID found in CIDs CSV")
				} else {
					if strings.TrimSpace(getCell("relationship:type", record)) == "" {
						panic("empty relationship type")
					}
					err = aaInstance.AddRelationship(
						cid,
						"parents",
						getCell("relationship:type", record),
						relCid,
					)
					if err != nil {
						panic(fmt.Errorf("adding relationship: %w", err))
					}
				}
			}
		}
	}
}
