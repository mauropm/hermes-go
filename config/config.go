package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const (
	DefaultHomeDirName    = ".hermes"
	DefaultMaxTurns       = 90
	DefaultTimeout        = 60 * time.Second
	DefaultMaxInputLen    = 100_000
	DefaultMaxRespLen     = 64 * 1024
	DefaultAPIPort        = 8080
	DefaultAPIHost        = "127.0.0.1"
	DefaultChatHistoryLen = 200
)

type Config struct {
	mu sync.RWMutex

	HomeDir string `yaml:"-"`

	Model          string   `yaml:"model"`
	Provider       string   `yaml:"provider"`
	FallbackModels []string `yaml:"fallback_providers"`
	Toolsets       []string `yaml:"toolsets"`

	Agent AgentConfig `yaml:"agent"`

	Terminal TerminalConfig `yaml:"terminal"`

	Memory    MemoryConfig    `yaml:"memory"`
	Security  SecurityConfig  `yaml:"security"`
	Privacy   PrivacyConfig   `yaml:"privacy"`
	APIServer APIServerConfig `yaml:"api_server"`
	Bedrock   BedrockConfig   `yaml:"bedrock"`
	Ollama    OllamaConfig    `yaml:"ollama"`

	APIKeys map[string]string `yaml:"-"`
}

type AgentConfig struct {
	MaxTurns    int    `yaml:"max_turns"`
	ToolUseMode string `yaml:"tool_use_enforcement"`
}

type TerminalConfig struct {
	Backend        string        `yaml:"backend"`
	Timeout        time.Duration `yaml:"timeout"`
	ChatHistoryLen int           `yaml:"chat_history_len"`
}

type MemoryConfig struct {
	Enabled       bool `yaml:"memory_enabled"`
	UserProfile   bool `yaml:"user_profile_enabled"`
	MemoryCharLim int  `yaml:"memory_char_limit"`
	UserCharLim   int  `yaml:"user_char_limit"`
}

type SecurityConfig struct {
	RedactSecrets bool `yaml:"redact_secrets"`
}

type PrivacyConfig struct {
	RedactPII bool `yaml:"redact_pii"`
}

type APIServerConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
	Key     string `yaml:"-"`
}

type BedrockConfig struct {
	Region          string `yaml:"region"`
	Profile         string `yaml:"profile"`
	BearerToken     string `yaml:"bearer_token,omitempty"`
	AccessKeyID     string `yaml:"access_key_id,omitempty"`
	SecretAccessKey string `yaml:"secret_access_key,omitempty"`
}

type OllamaConfig struct {
	BaseURL string        `yaml:"base_url"`
	Model   string        `yaml:"model"`
	Think   string        `yaml:"think"`
	Timeout time.Duration `yaml:"timeout"`
}

var (
	globalConfig *Config
	configOnce   sync.Once
	configErr    error
)

func Get() (*Config, error) {
	configOnce.Do(func() {
		globalConfig, configErr = Load("")
	})
	return globalConfig, configErr
}

func Load(profile string) (*Config, error) {
	homeDir, err := resolveHomeDir(profile)
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}

	cfg := defaultConfig(homeDir)

	if err := loadEnvFile(homeDir); err != nil {
		return nil, fmt.Errorf("load env file: %w", err)
	}

	cfg.APIKeys = loadAPIKeys()

	if err := loadYAMLConfig(homeDir, cfg); err != nil {
		return nil, fmt.Errorf("load yaml config: %w", err)
	}

	applyEnvOverrides(cfg)

	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func defaultConfig(homeDir string) *Config {
	return &Config{
		HomeDir:  homeDir,
		Model:    "anthropic/claude-sonnet-4-20250514",
		Provider: "auto",
		Toolsets: []string{"hermes-cli"},
		Agent: AgentConfig{
			MaxTurns:    DefaultMaxTurns,
			ToolUseMode: "auto",
		},
		Terminal: TerminalConfig{
			Backend:        "local",
			Timeout:        DefaultTimeout,
			ChatHistoryLen: DefaultChatHistoryLen,
		},
		Memory: MemoryConfig{
			Enabled:       false,
			UserProfile:   true,
			MemoryCharLim: 2200,
			UserCharLim:   1375,
		},
		Security: SecurityConfig{
			RedactSecrets: true,
		},
		Privacy: PrivacyConfig{
			RedactPII: false,
		},
		APIServer: APIServerConfig{
			Enabled: false,
			Host:    DefaultAPIHost,
			Port:    DefaultAPIPort,
		},
		Bedrock: BedrockConfig{
			Region:  "us-east-1",
			Profile: "",
		},
		Ollama: OllamaConfig{
			BaseURL: "http://localhost:11434",
			Model:   "llama3",
			Timeout: 300 * time.Second,
		},
		APIKeys: make(map[string]string),
	}
}

func resolveHomeDir(profile string) (string, error) {
	var base string
	if env := os.Getenv("HERMES_HOME"); env != "" {
		base = env
	} else if profile != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, DefaultHomeDirName, "profiles", profile)
	} else {
		if active, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), DefaultHomeDirName, "active_profile")); err == nil {
			p := strings.TrimSpace(string(active))
			if p != "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return "", err
				}
				base = filepath.Join(home, DefaultHomeDirName, "profiles", p)
			}
		}
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, DefaultHomeDirName)
		}
	}

	abs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}

	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			real = abs
		} else {
			return "", fmt.Errorf("eval symlinks: %w", err)
		}
	}

	return real, nil
}

func loadEnvFile(homeDir string) error {
	envPath := filepath.Join(homeDir, ".env")
	if _, err := os.Stat(envPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	info, err := os.Stat(envPath)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(envPath, 0o600); err != nil {
			return fmt.Errorf("restrict .env permissions: %w", err)
		}
	}

	return godotenv.Load(envPath)
}

func loadAPIKeys() map[string]string {
	keys := make(map[string]string)
	keyVars := []string{
		"OPENROUTER_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY",
		"GLM_API_KEY", "KIMI_API_KEY", "MINIMAX_API_KEY",
		"OPENCODE_ZEN_API_KEY", "HF_TOKEN", "DASHSCOPE_API_KEY",
		"API_SERVER_KEY",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
	}
	for _, v := range keyVars {
		if val := os.Getenv(v); val != "" {
			keys[v] = val
		}
	}
	return keys
}

func loadYAMLConfig(homeDir string, cfg *Config) error {
	configPath := filepath.Join(homeDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parse config.yaml: %w", err)
	}

	return nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("HERMES_MAX_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Agent.MaxTurns = n
		}
	}
	if v := os.Getenv("HERMES_YOLO_MODE"); v != "" {
		if v == "true" || v == "1" {
			cfg.Agent.ToolUseMode = "off"
		}
	}
	if v := os.Getenv("API_SERVER_ENABLED"); v != "" {
		if v == "true" || v == "1" {
			cfg.APIServer.Enabled = true
		}
	}
	if v := os.Getenv("API_SERVER_KEY"); v != "" {
		cfg.APIServer.Key = v
	}
	if v := os.Getenv("API_SERVER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.APIServer.Port = n
		}
	}
	if v := os.Getenv("API_SERVER_HOST"); v != "" {
		cfg.APIServer.Host = v
	}
}

func validateConfig(cfg *Config) error {
	if cfg.Agent.MaxTurns <= 0 || cfg.Agent.MaxTurns > 200 {
		return fmt.Errorf("max_turns must be between 1 and 200, got %d", cfg.Agent.MaxTurns)
	}
	if cfg.APIServer.Port < 1 || cfg.APIServer.Port > 65535 {
		return fmt.Errorf("api server port must be between 1 and 65535, got %d", cfg.APIServer.Port)
	}
	if cfg.APIServer.Enabled && cfg.APIServer.Key == "" {
		return fmt.Errorf("API server enabled but API_SERVER_KEY is not set")
	}
	return nil
}

func (cfg *Config) GetAPIKey(provider string) string {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	keyMap := map[string]string{
		"openai":     "OPENAI_API_KEY",
		"anthropic":  "ANTHROPIC_API_KEY",
		"openrouter": "OPENROUTER_API_KEY",
		"glm":        "GLM_API_KEY",
		"kimi":       "KIMI_API_KEY",
		"minimax":    "MINIMAX_API_KEY",
		"openzen":    "OPENCODE_ZEN_API_KEY",
		"hf":         "HF_TOKEN",
		"dashscope":  "DASHSCOPE_API_KEY",
	}

	if varName, ok := keyMap[strings.ToLower(provider)]; ok {
		return cfg.APIKeys[varName]
	}
	return ""
}

func (cfg *Config) GetAWSAccessKeyID() string {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	if cfg.Bedrock.AccessKeyID != "" {
		return cfg.Bedrock.AccessKeyID
	}
	return cfg.APIKeys["AWS_ACCESS_KEY_ID"]
}

func (cfg *Config) GetAWSSecretAccessKey() string {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	if cfg.Bedrock.SecretAccessKey != "" {
		return cfg.Bedrock.SecretAccessKey
	}
	return cfg.APIKeys["AWS_SECRET_ACCESS_KEY"]
}

func (cfg *Config) GetBedrockBearerToken() string {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()
	if cfg.Bedrock.BearerToken != "" {
		return cfg.Bedrock.BearerToken
	}
	return os.Getenv("AWS_BEARER_TOKEN_BEDROCK")
}

func (cfg *Config) EnsureDirs() error {
	dirs := []string{
		cfg.HomeDir,
		filepath.Join(cfg.HomeDir, "logs"),
		filepath.Join(cfg.HomeDir, "sessions"),
		filepath.Join(cfg.HomeDir, "memory"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("create dir %s: %w", d, err)
		}
	}
	return nil
}
