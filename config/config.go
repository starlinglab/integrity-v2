package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// See example_config.toml
type Config struct {
	AA struct {
		Url string `toml:"url"`
		Jwt string `toml:"jwt"`
	} `toml:"aa"`
	Webhook struct {
		Host string `toml:"host"`
		Jwt  string `toml:"jwt"`
	} `toml:"webhook"`
	Dirs struct {
		Files             string `toml:"files"`
		C2PAManifestTmpls string `toml:"c2pa_manifest_templates"`
		EncKeys           string `toml:"enc_keys"`
	} `toml:"dirs"`
	FolderPreprocessor struct {
		SyncFolderRoot string `toml:"sync_folder_root"`
	} `toml:"folder_preprocessor"`
	FolderDatabase struct {
		Host     string `toml:"host"`
		Port     string `toml:"port"`
		User     string `toml:"user"`
		Password string `toml:"password"`
		Database string `toml:"database"`
	} `toml:"folder_database"`
	Bins struct {
		Rclone   string `toml:"rclone"`
		C2patool string `toml:"c2patool"`
		W3       string `toml:"w3"`
	} `toml:"bins"`
	C2PA struct {
		PrivateKey string `toml:"private_key"`
		SignCert   string `toml:"sign_cert"`
	} `toml:"c2pa"`
	Numbers struct {
		Token              string `toml:"token"`
		NftContractAddress string `toml:"nft_contract_address"`
	} `toml:"numbers"`
	Browsertrix struct {
		User     string `toml:"user"`
		Password string `toml:"password"`
	} `toml:"browsertrix"`
}

var conf *Config

func GetConfig() *Config {
	if conf != nil {
		// Already loaded
		return conf
	}

	configPaths := []string{"/etc/integrity-v2/config.toml", "integrity-v2.toml"}
	if s := os.Getenv("INTEGRITY_CONFIG_PATH"); s != "" {
		// Force usage of env var over all else
		configPaths = []string{s}
	}

	var err error
	for _, path := range configPaths {
		_, err = toml.DecodeFile(path, &conf)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to decode config: %v\n", err)
			os.Exit(1)
		}
		// Decoding worked
		fmt.Fprintf(os.Stderr, "using config file: %s\n", path)
		break
	}

	if conf == nil {
		fmt.Fprintln(os.Stderr, "failed to find config file")
		os.Exit(1)
	}
	return conf
}
