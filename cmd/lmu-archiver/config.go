package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type config struct {
	DB         string   `json:"db"`
	RawDir     string   `json:"rawDir"`
	LMUURL     string   `json:"lmuUrl"`
	RaceOSURL  string   `json:"raceosUrl"`
	Listen     string   `json:"listen"`
	EventTypes []string `json:"eventTypes"`
}

// --config is accepted before the command or among its flags.
func loadConfig(args []string) (config, []string, error) {
	cfg := config{DB: "data/archive.sqlite3", RawDir: "data/raw", LMUURL: "http://127.0.0.1:6937", RaceOSURL: "https://raceos.gg", Listen: "127.0.0.1:8080", EventTypes: []string{"daily", "specialevent"}}
	var path string
	explicit := false
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		if args[i] == "--config" || strings.HasPrefix(args[i], "--config=") {
			if explicit {
				return cfg, nil, errors.New("--config must be specified once")
			}
			explicit = true
			if args[i] == "--config" {
				i++
				if i == len(args) {
					return cfg, nil, errors.New("--config requires a path")
				}
				path = args[i]
			} else {
				path = strings.TrimPrefix(args[i], "--config=")
			}
			if strings.TrimSpace(path) == "" {
				return cfg, nil, errors.New("--config requires a path")
			}
		} else {
			rest = append(rest, args[i])
		}
	}
	if !explicit {
		dir, err := os.UserConfigDir()
		if err != nil {
			return cfg, rest, nil
		}
		path = filepath.Join(dir, "lmu-archiver", "config.json")
	}
	file, err := os.Open(path)
	if !explicit && errors.Is(err, os.ErrNotExist) {
		return cfg, rest, nil
	}
	if err != nil {
		return cfg, nil, fmt.Errorf("read config: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	target := &cfg
	if err := decoder.Decode(&target); err != nil || target == nil {
		return cfg, nil, errors.New("config must be a JSON object with supported settings")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return cfg, nil, errors.New("config contains trailing JSON or invalid data")
	}
	return cfg, rest, nil
}
