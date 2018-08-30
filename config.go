package relay

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
)

type Config struct {
	ClientID     string `json:"clientId"`     // The Nest client ID.
	ClientSecret string `json:"clientSecret"` // The Nest client secret.
	ProjectID    string `json:"projectId"`    // The Firebase project ID.
}

func LoadConfig() *Config {
	path := os.Getenv("KLIMT_RELAY_CONFIG")
	if path == "" {
		log.Fatal("KLIMT_RELAY_CONFIG environment variable must point to config")
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("error opening config at %q: %s", path, err)
	}

	var cfg *Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		log.Fatalf("error parsing config %q: %s", data, err)
	}

	return cfg
}
