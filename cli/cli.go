package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nousresearch/hermes-go/core"
	"github.com/nousresearch/hermes-go/security"
)

const (
	promptText = "hermes> "
)

type CLI struct {
	agent   *core.Agent
	scanner *bufio.Scanner
}

func NewCLI(agent *core.Agent) *CLI {
	return &CLI{
		agent:   agent,
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
	default:
		fmt.Printf("Unknown command: %s. Type /help for available commands.\n", cmd)
		return nil
	}
}
