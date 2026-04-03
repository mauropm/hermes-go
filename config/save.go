package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

func (cfg *Config) Save() error {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()

	configPath := filepath.Join(cfg.HomeDir, "config.yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmpPath := configPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}

	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}

	return nil
}

func (cfg *Config) SetModel(model string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Model = model
}

func (cfg *Config) SetProvider(provider string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Provider = provider
}

func (cfg *Config) SetMaxTurns(n int) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Agent.MaxTurns = n
}

func (cfg *Config) SetMemoryEnabled(enabled bool) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Memory.Enabled = enabled
}

func (cfg *Config) SetBedrockRegion(region string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Bedrock.Region = region
}

func (cfg *Config) SetBedrockProfile(profile string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Bedrock.Profile = profile
}

func (cfg *Config) SetAPIPort(port int) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.APIServer.Port = port
}

func (cfg *Config) SetAPIHost(host string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.APIServer.Host = host
}

func (cfg *Config) SetAPIKeyEnabled(enabled bool) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	if enabled && cfg.APIServer.Key == "" {
		cfg.APIServer.Key = "change-me"
	} else if !enabled {
		cfg.APIServer.Key = ""
	}
}

func (cfg *Config) SetRedactSecrets(enabled bool) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Security.RedactSecrets = enabled
}

func (cfg *Config) SetRedactPII(enabled bool) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Privacy.RedactPII = enabled
}

func (cfg *Config) SetToolUseMode(mode string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Agent.ToolUseMode = mode
}

func (cfg *Config) SetBedrockAccessKey(key string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Bedrock.AccessKeyID = key
}

func (cfg *Config) SetBedrockSecretKey(key string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Bedrock.SecretAccessKey = key
}

func (cfg *Config) SetBedrockBearerToken(token string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Bedrock.BearerToken = token
}

func (cfg *Config) SetOllamaBaseURL(url string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Ollama.BaseURL = url
}

func (cfg *Config) SetOllamaModel(model string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Ollama.Model = model
}

func (cfg *Config) SetOllamaTimeout(timeout time.Duration) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Ollama.Timeout = timeout
}

func (cfg *Config) SetOllamaThink(think string) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Ollama.Think = think
}

func (cfg *Config) SetChatHistoryLen(n int) {
	cfg.mu.Lock()
	defer cfg.mu.Unlock()
	cfg.Terminal.ChatHistoryLen = n
}
