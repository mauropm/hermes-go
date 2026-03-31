package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/nousresearch/hermes-go/api"
	"github.com/nousresearch/hermes-go/cli"
	"github.com/nousresearch/hermes-go/config"
	"github.com/nousresearch/hermes-go/core"
	"github.com/nousresearch/hermes-go/llm"
	"github.com/nousresearch/hermes-go/memory"
	"github.com/nousresearch/hermes-go/storage"
	"github.com/nousresearch/hermes-go/tools"
)

const version = "0.6.0"

func main() {
	var (
		profile  string
		cmd      string
		showVer  bool
		thinking bool
	)

	flag.StringVar(&profile, "p", "", "Profile name")
	flag.StringVar(&profile, "profile", "", "Profile name")
	flag.StringVar(&cmd, "cmd", "", "Command to run (chat, api)")
	flag.BoolVar(&showVer, "version", false, "Show version")
	flag.BoolVar(&showVer, "v", false, "Show version")
	flag.BoolVar(&thinking, "thinking", false, "Show model thinking/reasoning output")
	flag.Parse()

	if showVer {
		fmt.Printf("hermes-go %s\n", version)
		os.Exit(0)
	}

	if cmd == "" {
		args := flag.Args()
		if len(args) > 0 {
			cmd = args[0]
		} else {
			cmd = "chat"
		}
	}

	cfg, err := config.Load(profile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("Failed to create directories: %v", err)
	}

	log.SetOutput(os.Stderr)
	log.SetPrefix("[hermes] ")
	log.SetFlags(log.Ldate | log.Ltime)

	switch cmd {
	case "chat":
		runChat(cfg, thinking)
	case "api", "gateway":
		runAPI(cfg)
	case "setup":
		runSetup(cfg)
	case "version":
		fmt.Printf("hermes-go %s\n", version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		fmt.Fprintf(os.Stderr, "Usage: hermes-go [chat|api|setup] [-p profile] [-thinking]\n")
		os.Exit(1)
	}
}

func runChat(cfg *config.Config, showThinking bool) {
	sessionDB, err := storage.NewSessionDB(cfg.HomeDir)
	if err != nil {
		log.Fatalf("Failed to initialize session database: %v", err)
	}
	defer sessionDB.Close()

	toolRegistry := tools.NewRegistry()
	if err := tools.RegisterBuiltinTools(toolRegistry); err != nil {
		log.Fatalf("Failed to register tools: %v", err)
	}

	sessionID := uuid.New().String()

	var memStore *memory.Store
	if cfg.Memory.Enabled {
		memStore, err = memory.NewStore(cfg.HomeDir, 30*24*time.Hour)
		if err != nil {
			log.Printf("Warning: failed to initialize memory store: %v", err)
		}
	}

	provider := cfg.Provider
	if provider == "auto" {
		provider = llm.DetectProvider(cfg.Model)
	}

	var apiKey string
	if provider != "bedrock" {
		apiKey = cfg.GetAPIKey(provider)
		if apiKey == "" {
			log.Fatalf("No API key found for provider %q. Set the appropriate environment variable (e.g., OPENAI_API_KEY, ANTHROPIC_API_KEY).", provider)
		}
	}

	agent, err := core.NewAgent(core.AgentConfig{
		Model:              cfg.Model,
		Provider:           provider,
		APIKey:             apiKey,
		BedrockBearerToken: cfg.GetBedrockBearerToken(),
		BedrockAccessKey:   cfg.GetAWSAccessKeyID(),
		BedrockSecretKey:   cfg.GetAWSSecretAccessKey(),
		ToolRegistry:       toolRegistry,
		SessionDB:          sessionDB,
		MemStore:           memStore,
		MaxTurns:           cfg.Agent.MaxTurns,
		SessionID:          sessionID,
		Source:             "cli",
		ShowThinking:       showThinking,
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	if err := sessionDB.CreateSession(sessionID, "cli", "", cfg.Model, "", core.BuildSystemPrompt("")); err != nil {
		log.Printf("Warning: failed to create session record: %v", err)
	}

	c := cli.NewCLI(agent, cfg)

	if err := c.Run(); err != nil {
		log.Printf("Session ended: %v", err)
	}

	if err := agent.SaveSession(); err != nil {
		log.Printf("Warning: failed to save session: %v", err)
	}

	if err := sessionDB.EndSession(sessionID, "user_exit"); err != nil {
		log.Printf("Warning: failed to end session: %v", err)
	}
}

func runAPI(cfg *config.Config) {
	sessionDB, err := storage.NewSessionDB(cfg.HomeDir)
	if err != nil {
		log.Fatalf("Failed to initialize session database: %v", err)
	}
	defer sessionDB.Close()

	toolRegistry := tools.NewRegistry()
	if err := tools.RegisterBuiltinTools(toolRegistry); err != nil {
		log.Fatalf("Failed to register tools: %v", err)
	}

	sessionID := uuid.New().String()

	provider := cfg.Provider
	if provider == "auto" {
		provider = llm.DetectProvider(cfg.Model)
	}

	var apiKey string
	if provider != "bedrock" {
		apiKey = cfg.GetAPIKey(provider)
		if apiKey == "" {
			log.Fatalf("No API key found for provider %q. Set the appropriate environment variable.", provider)
		}
	}

	agent, err := core.NewAgent(core.AgentConfig{
		Model:              cfg.Model,
		Provider:           provider,
		APIKey:             apiKey,
		BedrockBearerToken: cfg.GetBedrockBearerToken(),
		BedrockAccessKey:   cfg.GetAWSAccessKeyID(),
		BedrockSecretKey:   cfg.GetAWSSecretAccessKey(),
		ToolRegistry:       toolRegistry,
		SessionDB:          sessionDB,
		MaxTurns:           cfg.Agent.MaxTurns,
		SessionID:          sessionID,
		Source:             "api",
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	srv := api.NewServer(agent, cfg.APIServer.Key, cfg.APIServer.Host, cfg.APIServer.Port)

	if cfg.APIServer.Host != "127.0.0.1" && cfg.APIServer.Host != "localhost" {
		log.Printf("WARNING: API server is bound to %s (not localhost). Bearer tokens and conversation data will be transmitted in plaintext. Configure TLS to secure connections.", cfg.APIServer.Host)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("API server starting on %s:%d", cfg.APIServer.Host, cfg.APIServer.Port)
		if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Fatalf("API server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down API server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped.")
}

func runSetup(cfg *config.Config) {
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║       Hermes Setup Wizard                ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("Config directory: %s\n", cfg.HomeDir)
	fmt.Println()

	tui := cli.NewConfigTUI(cfg)
	if err := tui.Run(); err != nil {
		log.Fatalf("Setup failed: %v", err)
	}
}
