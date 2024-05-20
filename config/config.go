package config

import (
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
		C2PA              string `toml:"c2pa"`
		C2PAManifestTmpls string `toml:"c2pa_manifest_templates"`
		MetadataEncKeys   string `toml:"metadata_enc_keys"`
		FileEncKeys       string `toml:"file_enc_keys"`
	} `toml:"dirs"`
	FolderPreprocessor struct {
		SyncFolderRoot string   `toml:"sync_folder_root"`
		FileExtensions []string `toml:"file_extensions"`
	} `toml:"folder_preprocessor"`
	Bins struct {
		Ipfs     string `toml:"ipfs"`
		Rclone   string `toml:"rclone"`
		C2patool string `toml:"c2patool"`
		W3       string `toml:"w3"`
	} `toml:"bins"`
	C2PA struct {
		PrivateKey string `toml:"private_key"`
		SignCert   string `toml:"sign_cert"`
	} `toml:"c2pa"`
}

var conf *Config

func GetConfig() *Config {
	if conf != nil {
		// Already loaded
		return conf
	}

	configPath := os.Getenv("INTEGRITY_CONFIG_PATH") // For debugging
	if configPath == "" {
		// Default well known path
		configPath = "/etc/integrity-v2/config.toml"
	}
	_, err := toml.DecodeFile(configPath, &conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode config: %v\n", err)
		os.Exit(1)
	}
	return conf
}
