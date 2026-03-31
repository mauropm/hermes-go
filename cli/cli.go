package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/nousresearch/hermes-go/config"
	"github.com/nousresearch/hermes-go/core"
	"github.com/nousresearch/hermes-go/llm"
	"github.com/nousresearch/hermes-go/security"
)

const (
	promptText = "hermes> "
)

type CLI struct {
	agent   *core.Agent
	cfg     *config.Config
	scanner *bufio.Scanner
}

func NewCLI(agent *core.Agent, cfg *config.Config) *CLI {
	return &CLI{
		agent:   agent,
		cfg:     cfg,
		scanner: bufio.NewScanner(os.Stdin),
	}
}

func (c *CLI) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nInterrupting...")
		c.agent.Interrupt()
		cancel()
	}()

	fmt.Println("Hermes Go - Secure AI Assistant")
	fmt.Println("Type /quit or /exit to leave. Type /help for commands.")
	fmt.Println()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		fmt.Print(promptText)
		if !c.scanner.Scan() {
			break
		}

		input := c.scanner.Text()
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		if strings.HasPrefix(input, "/") {
			if err := c.handleCommand(ctx, input); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			}
			continue
		}

		if err := security.ValidateLength(input, security.MaxInputLength); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		response, err := c.agent.Chat(ctx, input)
		if err != nil {
			if err == context.Canceled {
				fmt.Println("\nCancelled.")
				continue
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			continue
		}

		fmt.Println()
		fmt.Println(response)
		fmt.Println()
	}

	return nil
}

func (c *CLI) handleCommand(ctx context.Context, input string) error {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/quit", "/exit":
		if err := c.agent.SaveSession(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to save session: %v\n", err)
		}
		os.Exit(0)
		return nil
	case "/help":
		fmt.Println("Available commands:")
		fmt.Println("  /quit, /exit  - Exit the application")
		fmt.Println("  /help         - Show this help message")
		fmt.Println("  /session      - Show current session ID")
		fmt.Println("  /tools        - List available tools")
		fmt.Println("  /models       - List and select Bedrock models")
		fmt.Println("  /config       - Open configuration editor")
		fmt.Println("  /clear        - Clear the screen")
		return nil
	case "/session":
		fmt.Printf("Session ID: %s\n", c.agent.GetSessionID())
		return nil
	case "/tools":
		tools := c.agent.GetToolDefinitions()
		fmt.Printf("Available tools (%d):\n", len(tools))
		for _, t := range tools {
			fmt.Printf("  - %s: %s\n", t.Function.Name, t.Function.Description)
		}
		return nil
	case "/clear":
		fmt.Print("\033[H\033[2J")
		return nil
	case "/config":
		tui := NewConfigTUI(c.cfg)
		if err := tui.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		}
		return nil
	case "/models":
		c.handleModels(parts)
		return nil
	default:
		fmt.Printf("Unknown command: %s. Type /help for available commands.\n", cmd)
		return nil
	}
}

func (c *CLI) handleModels(parts []string) {
	if len(parts) < 2 {
		c.printBedrockModels()
		return
	}

	sub := strings.ToLower(parts[1])

	switch sub {
	case "list":
		c.printBedrockModels()
	case "use":
		if len(parts) < 3 {
			fmt.Println("Usage: /models use <number>")
			fmt.Println("Run /models list to see available models.")
			return
		}
		idx, err := strconv.Atoi(parts[2])
		if err != nil {
			fmt.Printf("Invalid model number: %s\n", parts[2])
			return
		}
		c.selectBedrockModel(idx)
	default:
		if num, err := strconv.Atoi(sub); err == nil {
			c.selectBedrockModel(num)
		} else {
			fmt.Printf("Unknown subcommand: %s. Use /models list or /models use <number>\n", sub)
		}
	}
}

func (c *CLI) printBedrockModels() {
	models := llm.BedrockModels()

	fmt.Println()
	fmt.Println("Available AWS Bedrock Models:")
	fmt.Println(strings.Repeat("-", 95))
	fmt.Printf("%-4s %-30s %-12s %-12s %-12s %-8s\n", "#", "Model", "Input $/MTok", "Output $/MTok", "Context", "Cost")
	fmt.Println(strings.Repeat("-", 95))

	for i, m := range models {
		costLabel := "$$$"
		if m.InputCost <= 0.20 {
			costLabel = "$"
		} else if m.InputCost <= 1.00 {
			costLabel = "$$"
		}
		freeLabel := ""
		if m.IsFree() {
			freeLabel = " [FREE]"
		}
		fmt.Printf("%-4d %-30s $%-11.2f $%-11.2f %-12d %-8s%s\n",
			i+1, m.Name, m.InputCost, m.OutputCost, m.ContextLen, costLabel, freeLabel)
	}

	fmt.Println(strings.Repeat("-", 95))
	fmt.Println()
	fmt.Println("Select a model: /models use <number>  (e.g., /models use 1)")
	fmt.Println()
}

func (c *CLI) selectBedrockModel(idx int) {
	models := llm.BedrockModels()

	if idx < 1 || idx > len(models) {
		fmt.Printf("Invalid model number. Must be between 1 and %d.\n", len(models))
		return
	}

	m := models[idx-1]

	err := c.agent.SetModel(m.ID, "bedrock", "", "", c.cfg.Bedrock.Region, c.cfg.Bedrock.Profile)
	if err != nil {
		fmt.Printf("Failed to switch model: %v\n", err)
		return
	}

	costLabel := "paid"
	if m.IsFree() {
		costLabel = "free tier"
	}

	fmt.Printf("Switched to %s (%s)\n", m.Name, costLabel)
	fmt.Printf("  Provider:  %s\n", m.Provider)
	fmt.Printf("  Input:     $%.2f/M tokens\n", m.InputCost)
	fmt.Printf("  Output:    $%.2f/M tokens\n", m.OutputCost)
	fmt.Printf("  Context:   %d tokens\n", m.ContextLen)
	fmt.Printf("  Region:    %s\n", c.cfg.Bedrock.Region)
	fmt.Println()
}
