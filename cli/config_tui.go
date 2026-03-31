package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nousresearch/hermes-go/config"
	"github.com/nousresearch/hermes-go/llm"
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
		if err != nil || choice < 0 || choice > 12 {
			fmt.Println("Invalid choice. Enter a number 0-12.")
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
	fmt.Printf("║  1. Model          │ %-29s ║\n", t.shortModel())
	fmt.Printf("║  2. Max Turns      │ %-29d ║\n", t.cfg.Agent.MaxTurns)
	fmt.Printf("║  3. Memory         │ %-29t ║\n", t.cfg.Memory.Enabled)
	fmt.Printf("║  4. Redact Secrets │ %-29t ║\n", t.cfg.Security.RedactSecrets)
	fmt.Printf("║  5. Redact PII     │ %-29t ║\n", t.cfg.Privacy.RedactPII)
	fmt.Printf("║  6. Bedrock Region │ %-29s ║\n", t.cfg.Bedrock.Region)
	fmt.Printf("║  7. Bedrock Profile│ %-29s ║\n", t.cfg.Bedrock.Profile)
	fmt.Printf("║  8. AWS Access Key │ %-29s ║\n", t.maskedKey(t.cfg.Bedrock.AccessKeyID))
	fmt.Printf("║  9. AWS Secret Key │ %-29s ║\n", t.maskedKey(t.cfg.Bedrock.SecretAccessKey))
	fmt.Printf("║ 10. API Host       │ %-29s ║\n", t.cfg.APIServer.Host)
	fmt.Printf("║ 11. API Port       │ %-29d ║\n", t.cfg.APIServer.Port)
	fmt.Printf("║ 12. API Key Req'd  │ %-29t ║\n", t.cfg.APIServer.Key != "")
	fmt.Println("╠══════════════════════════════════════════╣")
	fmt.Println("║  0. Save & Exit                          ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Print("\nSelect option (0-12): ")
}

func (t *ConfigTUI) maskedKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

func (t *ConfigTUI) shortModel() string {
	m := t.cfg.Model
	if len(m) > 29 {
		return m[:26] + "..."
	}
	return m
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
		t.selectModel()

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
		fmt.Print("AWS Access Key ID: ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		if val != "" {
			t.cfg.SetBedrockAccessKey(val)
		}

	case 9:
		fmt.Print("AWS Secret Access Key: ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		if val != "" {
			t.cfg.SetBedrockSecretKey(val)
		}

	case 10:
		fmt.Print("API server host (current: " + t.cfg.APIServer.Host + "): ")
		if !t.scanner.Scan() {
			return false, nil
		}
		val := strings.TrimSpace(t.scanner.Text())
		if val != "" {
			t.cfg.SetAPIHost(val)
		}

	case 11:
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

	case 12:
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

func (t *ConfigTUI) selectModel() {
	fmt.Println()
	fmt.Println("Select a model provider:")
	fmt.Println("  1. AWS Bedrock  (uses AWS credentials)")
	fmt.Println("  2. Anthropic    (direct API)")
	fmt.Println("  3. OpenAI       (direct API)")
	fmt.Println("  4. Custom       (manual model ID)")
	fmt.Print("\nChoice (1-4): ")
	if !t.scanner.Scan() {
		return
	}
	provChoice := strings.TrimSpace(t.scanner.Text())

	switch provChoice {
	case "1":
		t.selectBedrockModel()
	case "2":
		t.cfg.SetModel("anthropic/claude-sonnet-4-20250514")
		fmt.Println("Set to: anthropic/claude-sonnet-4-20250514")
	case "3":
		t.cfg.SetModel("openai/gpt-4o")
		fmt.Println("Set to: openai/gpt-4o")
	case "4":
		fmt.Print("Enter model ID (e.g., anthropic/claude-sonnet-4-20250514): ")
		if !t.scanner.Scan() {
			return
		}
		val := strings.TrimSpace(t.scanner.Text())
		if val != "" {
			t.cfg.SetModel(val)
		}
	default:
		fmt.Println("Invalid choice.")
	}
}

func (t *ConfigTUI) selectBedrockModel() {
	models := llm.BedrockModels()

	fmt.Println()
	fmt.Println("AWS Bedrock Models:")
	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("%-4s %-28s %-10s %-8s\n", "#", "Model", "Input$/MT", "Cost")
	fmt.Println(strings.Repeat("-", 70))

	for i, m := range models {
		costLabel := "$$$"
		if m.IsFree() {
			costLabel = "FREE"
		} else if m.InputCost <= 0.20 {
			costLabel = "$"
		} else if m.InputCost <= 1.00 {
			costLabel = "$$"
		}
		fmt.Printf("%-4d %-28s $%-9.2f %-8s\n",
			i+1, m.Name, m.InputCost, costLabel)
	}

	fmt.Println(strings.Repeat("-", 70))
	fmt.Printf("\nCurrent: %s\n", t.cfg.Model)
	fmt.Print("\nSelect model number (or 0 to cancel): ")
	if !t.scanner.Scan() {
		return
	}
	val := strings.TrimSpace(t.scanner.Text())

	idx, err := strconv.Atoi(val)
	if err != nil || idx < 0 || idx > len(models) {
		fmt.Println("Invalid selection.")
		return
	}
	if idx == 0 {
		return
	}

	m := models[idx-1]
	t.cfg.SetModel(m.ID)

	costLabel := "paid"
	if m.IsFree() {
		costLabel = "free tier"
	}

	fmt.Printf("\nSelected: %s (%s)\n", m.Name, costLabel)
	fmt.Printf("  Provider: %s\n", m.Provider)
	fmt.Printf("  Input:    $%.2f/M tokens\n", m.InputCost)
	fmt.Printf("  Output:   $%.2f/M tokens\n", m.OutputCost)
	fmt.Printf("  Context:  %d tokens\n", m.ContextLen)
}
