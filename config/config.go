package config

import (
	"os"

	"github.com/BurntSushi/toml"
	"github.com/starlinglab/integrity-v2/util"
)

// See example_config.toml
type Config struct {
	AA struct {
		Url string `toml:"url"`
		Jwt string `toml:"jwt"`
	} `toml:"aa"`
	Webhook struct {
		Host string `toml:"host"`
	} `toml:"webhook"`
	Dirs struct {
		Files           string `toml:"files"`
		C2PA            string `toml:"c2pa"`
		C2PAManifests   string `toml:"c2pa_manifests"`
		MetadataEncKeys string `toml:"metadata_enc_keys"`
		FileEncKeys     string `toml:"file_enc_keys"`
	} `toml:"dirs"`
	Bins struct {
		Ipfs   string `toml:"ipfs"`
		Rclone string `toml:"rclone"`
	} `toml:"bins"`
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
		util.Die("failed to decode config (%s): %v", configPath, err)
	}
	return conf
}
