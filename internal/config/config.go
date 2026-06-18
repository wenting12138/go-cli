package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dirName  = ".go-cli"
	fileName = "config.json"
)

type Config struct {
	CurrentModel string        `json:"current_model"`
	Models       []ModelConfig `json:"models"`
}

type ModelConfig struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	ContextWindow int    `json:"context_window"`
	MaxTokens     int    `json:"max_tokens"`
}

type State struct {
	Path   string
	Config Config
}

type legacyConfig struct {
	Model ModelConfig `json:"model"`
}

func Ensure() (State, error) {
	configPath, err := defaultPath()
	if err != nil {
		return State{}, err
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return State{}, fmt.Errorf("create config dir: %w", err)
	}

	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		cfg := defaultConfig()
		if err := write(configPath, cfg); err != nil {
			return State{}, err
		}
		return State{Path: configPath, Config: cfg}, nil
	} else if err != nil {
		return State{}, fmt.Errorf("stat config file: %w", err)
	}

	cfg, migrated, err := load(configPath)
	if err != nil {
		return State{}, err
	}
	if migrated {
		if err := write(configPath, cfg); err != nil {
			return State{}, err
		}
	}

	return State{Path: configPath, Config: cfg}, nil
}

func (s *State) ActiveModel() (ModelConfig, error) {
	for _, model := range s.Config.Models {
		if model.ID == s.Config.CurrentModel {
			return model, nil
		}
	}
	return ModelConfig{}, fmt.Errorf("active model %q not found", s.Config.CurrentModel)
}

func (s *State) SelectModel(id string) (ModelConfig, error) {
	for _, model := range s.Config.Models {
		if model.ID == id {
			s.Config.CurrentModel = id
			if err := write(s.Path, s.Config); err != nil {
				return ModelConfig{}, err
			}
			return model, nil
		}
	}
	return ModelConfig{}, fmt.Errorf("model %q not found", id)
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, dirName, fileName), nil
}

func defaultConfig() Config {
	return Config{
		CurrentModel: "openai-gpt-4.1-mini",
		Models: []ModelConfig{
			{
				ID:            "openai-gpt-4.1-mini",
				Provider:      "openai",
				Name:          "gpt-4.1-mini",
				BaseURL:       "https://api.openai.com/v1",
				APIKey:        "",
				ContextWindow: 128000,
				MaxTokens:     2048,
			},
			{
				ID:            "openai-gpt-4.1",
				Provider:      "openai",
				Name:          "gpt-4.1",
				BaseURL:       "https://api.openai.com/v1",
				APIKey:        "",
				ContextWindow: 128000,
				MaxTokens:     4096,
			},
			{
				ID:            "ollama-qwen3",
				Provider:      "ollama",
				Name:          "qwen3:latest",
				BaseURL:       "http://localhost:11434",
				APIKey:        "",
				ContextWindow: 32000,
				MaxTokens:     2048,
			},
		},
	}
}

func load(path string) (Config, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, false, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err == nil && len(cfg.Models) > 0 {
		if err := validate(cfg); err != nil {
			return Config{}, false, err
		}
		return cfg, false, nil
	}

	var legacy legacyConfig
	if err := json.Unmarshal(data, &legacy); err != nil {
		return Config{}, false, fmt.Errorf("parse config file %s: %w", path, err)
	}
	if legacy.Model.Provider == "" || legacy.Model.Name == "" {
		return Config{}, false, fmt.Errorf("config file %s is missing models", path)
	}

	if legacy.Model.ID == "" {
		legacy.Model.ID = fmt.Sprintf("%s-%s", legacy.Model.Provider, legacy.Model.Name)
	}
	cfg = Config{
		CurrentModel: legacy.Model.ID,
		Models:       []ModelConfig{legacy.Model},
	}
	if err := validate(cfg); err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

func validate(cfg Config) error {
	if len(cfg.Models) == 0 {
		return fmt.Errorf("config file is missing models")
	}
	if cfg.CurrentModel == "" {
		return fmt.Errorf("config file is missing current_model")
	}

	seen := map[string]struct{}{}
	currentFound := false
	for _, model := range cfg.Models {
		if model.ID == "" {
			return fmt.Errorf("config file has a model with missing id")
		}
		if _, exists := seen[model.ID]; exists {
			return fmt.Errorf("config file has duplicated model id %q", model.ID)
		}
		seen[model.ID] = struct{}{}
		if model.ID == cfg.CurrentModel {
			currentFound = true
		}
		if model.Provider == "" {
			return fmt.Errorf("config file model %q is missing provider", model.ID)
		}
		if model.Name == "" {
			return fmt.Errorf("config file model %q is missing name", model.ID)
		}
		if model.ContextWindow <= 0 {
			return fmt.Errorf("config file model %q has invalid context_window", model.ID)
		}
		if model.MaxTokens <= 0 {
			return fmt.Errorf("config file model %q has invalid max_tokens", model.ID)
		}
	}
	if !currentFound {
		return fmt.Errorf("config file current_model %q not found", cfg.CurrentModel)
	}
	return nil
}

func write(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}
