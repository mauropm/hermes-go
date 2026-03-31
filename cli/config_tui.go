package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nousresearch/hermes-go/config"
)

type ConfigTUI struct {
	cfg     *config.Config
	scanner *bufio.Scanner
}

func NewConfigTUI(cfg *config.Config) *ConfigTUI {
	return &ConfigTUI{
		cfg:     cfg,
		scanner: bufio.NewScanner(os.Stdin),
	}
}

func (t *ConfigTUI) Run() error {
	for {
		t.printMenu()
		if !t.scanner.Scan() {
			return nil
		}
		input := strings.TrimSpace(t.scanner.Text())
		if input == "" {
			continue
		}

		choice, err := strconv.Atoi(input)
		if err != nil || choice < 0 || choice > 10 {
			fmt.Println("Invalid choice. Enter a number 0-10.")
			continue
		}

		stop, err := t.handleChoice(choice)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		if stop {
			return nil
		}
	}
}

func (t *ConfigTUI) printMenu() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║         Hermes Configuration             ║")
	fmt.Println("╠══════════════════════════════════════════╣")
	fmt.Printf("║  1. Model          │ %-29s ║\n", t.cfg.Model)
	fmt.Printf("║  2. Max Turns      │ %-29d ║\n", t.cfg.Agent.MaxTurns)
	fmt.Printf("║  3. Memory         │ %-29t ║\n", t.cfg.Memory.Enabled)
	fmt.Printf("║  4. Redact Secrets │ %-29t ║\n", t.cfg.Security.RedactSecrets)
	fmt.Printf("║  5. Redact PII     │ %-29t ║\n", t.cfg.Privacy.RedactPII)
	fmt.Printf("║  6. Bedrock Region │ %-29s ║\n", t.cfg.Bedrock.Region)
	fmt.Printf("║  7. Bedrock Profile│ %-29s ║\n", t.cfg.Bedrock.Profile)
	fmt.Printf("║  8. API Host       │ %-29s ║\n", t.cfg.APIServer.Host)
	fmt.Printf("║  9. API Port       │ %-29d ║\n", t.cfg.APIServer.Port)
	fmt.Printf("║ 10. API Key Req'd  │ %-29t ║\n", t.cfg.APIServer.Key != "")
	fmt.Println("╠══════════════════════════════════════════╣")
	fmt.Println("║  0. Save & Exit                          ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Print("\nSelect option (0-10): ")
}

func (t *ConfigTUI) handleChoice(choice int) (bool, error) {
	switch choice {
	case 0:
		if err := t.cfg.Save(); err != nil {
			return false, fmt.Errorf("save config: %w", err)
		}
		fmt.Println("Configuration saved to config.yaml.")
		return true, nil

	case 1:
		fmt.Println()
		fmt.Println("Set model (e.g., anthropic/claude-sonnet-4-20250514):")
		fmt.Println("  Popular choices:")
		fmt.Println("    anthropic/claude-sonnet-4-20250514")
		fmt.Println("    anthropic/claude-opus-4-20250514")
		fmt.Println("    openai/gpt-4o")
		fmt.Println("  Or type a Bedrock model ID from /models list.")
		fmt.Print("  Model: ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		if val != "" {
			t.cfg.SetModel(val)
		}

	case 2:
		fmt.Print("Max turns (1-200): ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		if n, err := strconv.Atoi(val); err == nil && n >= 1 && n <= 200 {
			t.cfg.SetMaxTurns(n)
		} else {
			fmt.Println("Invalid value. Must be 1-200.")
		}

	case 3:
		fmt.Printf("Enable memory? (current: %t) [y/n]: ", t.cfg.Memory.Enabled)
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(strings.ToLower(t.scanner.Text()))
		if val == "y" || val == "yes" {
			t.cfg.SetMemoryEnabled(true)
		} else if val == "n" || val == "no" {
			t.cfg.SetMemoryEnabled(false)
		}

	case 4:
		fmt.Printf("Redact secrets in logs? (current: %t) [y/n]: ", t.cfg.Security.RedactSecrets)
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(strings.ToLower(t.scanner.Text()))
		if val == "y" || val == "yes" {
			t.cfg.SetRedactSecrets(true)
		} else if val == "n" || val == "no" {
			t.cfg.SetRedactSecrets(false)
		}

	case 5:
		fmt.Printf("Redact PII? (current: %t) [y/n]: ", t.cfg.Privacy.RedactPII)
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(strings.ToLower(t.scanner.Text()))
		if val == "y" || val == "yes" {
			t.cfg.SetRedactPII(true)
		} else if val == "n" || val == "no" {
			t.cfg.SetRedactPII(false)
		}

	case 6:
		fmt.Print("Bedrock region (e.g., us-east-1, us-west-2): ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		if val != "" {
			t.cfg.SetBedrockRegion(val)
		}

	case 7:
		fmt.Print("Bedrock AWS profile (leave empty for default): ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		t.cfg.SetBedrockProfile(val)

	case 8:
		fmt.Print("API server host (current: " + t.cfg.APIServer.Host + "): ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		if val != "" {
			t.cfg.SetAPIHost(val)
		}

	case 9:
		fmt.Print("API server port (current: " + strconv.Itoa(t.cfg.APIServer.Port) + "): ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		if n, err := strconv.Atoi(val); err == nil && n >= 1 && n <= 65535 {
			t.cfg.SetAPIPort(n)
		} else if val != "" {
			fmt.Println("Invalid port. Must be 1-65535.")
		}

	case 10:
		fmt.Printf("Require API key for API server? (current: %t) [y/n]: ", t.cfg.APIServer.Key != "")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(strings.ToLower(t.scanner.Text()))
		if val == "y" || val == "yes" {
			t.cfg.SetAPIKeyEnabled(true)
		} else if val == "n" || val == "no" {
			t.cfg.SetAPIKeyEnabled(false)
		}
	}

	return false, nil
}
