package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nousresearch/hermes-go/config"
	"github.com/nousresearch/hermes-go/core"
	"github.com/nousresearch/hermes-go/llm"
	"github.com/nousresearch/hermes-go/security"

	"golang.org/x/term"
)

const (
	promptText = "hermes> "
)

type CLI struct {
	agent        *core.Agent
	cfg          *config.Config
	history      []string
	historyIndex int
	oldState     *term.State
}

func NewCLI(agent *core.Agent, cfg *config.Config) *CLI {
	return &CLI{
		agent:        agent,
		cfg:          cfg,
		history:      make([]string, 0),
		historyIndex: -1,
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

	// Save and restore terminal state
	if term.IsTerminal(int(os.Stdin.Fd())) {
		var err error
		c.oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("failed to set terminal to raw mode: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), c.oldState)
	}

	fmt.Println("Hermes Go - Secure AI Assistant")
	fmt.Println("Type /quit or /exit to leave. Type /help for commands.")
	fmt.Println("Use Up/Down arrows to navigate command history.")
	fmt.Println()

	currentInput := ""
	cursorPos := 0

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Print prompt and current input
		fmt.Print("\r" + promptText + currentInput)
		fmt.Print("\033[K") // Clear to end of line
		if cursorPos < len(currentInput) {
			// Move cursor back to correct position
			fmt.Printf("\033[%dG", len(promptText)+cursorPos+1)
		}

		// Read single character
		b := make([]byte, 1)
		n, err := os.Stdin.Read(b)
		if err != nil || n == 0 {
			break
		}

		ch := b[0]

		// Handle escape sequences (arrow keys)
		if ch == 27 { // ESC
			// Read next characters for escape sequence
			seq := make([]byte, 2)
			os.Stdin.Read(seq)

			if seq[0] == 91 { // [
				switch seq[1] {
				case 65: // Up arrow
					if len(c.history) > 0 {
						if c.historyIndex == -1 {
							c.historyIndex = len(c.history) - 1
						} else if c.historyIndex > 0 {
							c.historyIndex--
						}
						currentInput = c.history[c.historyIndex]
						cursorPos = len(currentInput)
					}
				case 66: // Down arrow
					if c.historyIndex != -1 {
						if c.historyIndex < len(c.history)-1 {
							c.historyIndex++
							currentInput = c.history[c.historyIndex]
						} else {
							c.historyIndex = -1
							currentInput = ""
						}
						cursorPos = len(currentInput)
					}
				case 67: // Right arrow
					if cursorPos < len(currentInput) {
						cursorPos++
					}
				case 68: // Left arrow
					if cursorPos > 0 {
						cursorPos--
					}
				case 51: // Delete (DEL)
					if cursorPos < len(currentInput) {
						currentInput = currentInput[:cursorPos] + currentInput[cursorPos+1:]
					}
				}
				continue
			}
		}

		// Handle special characters
		switch ch {
		case 13: // Enter
			fmt.Println()
			input := strings.TrimSpace(currentInput)

			if input == "" {
				currentInput = ""
				cursorPos = 0
				continue
			}

			// Add to history
			if len(c.history) == 0 || c.history[len(c.history)-1] != input {
				c.history = append(c.history, input)
				// Limit history size
				maxHistory := c.cfg.Terminal.ChatHistoryLen
				if maxHistory <= 0 {
					maxHistory = config.DefaultChatHistoryLen
				}
				if len(c.history) > maxHistory {
					c.history = c.history[len(c.history)-maxHistory:]
				}
			}
			c.historyIndex = -1

			if strings.HasPrefix(input, "/") {
				if err := c.handleCommand(ctx, input); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				}
				currentInput = ""
				cursorPos = 0
				continue
			}

			if err := security.ValidateLength(input, security.MaxInputLength); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				currentInput = ""
				cursorPos = 0
				continue
			}

			response, err := c.agent.Chat(ctx, input)
			if err != nil {
				if err == context.Canceled {
					fmt.Println("\nCancelled.")
					currentInput = ""
					cursorPos = 0
					continue
				}
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				currentInput = ""
				cursorPos = 0
				continue
			}

			fmt.Println()
			fmt.Println(response)
			fmt.Println()

			currentInput = ""
			cursorPos = 0

		case 127, 8: // Backspace
			if cursorPos > 0 {
				currentInput = currentInput[:cursorPos-1] + currentInput[cursorPos:]
				cursorPos--
			}

		case 3: // Ctrl+C
			fmt.Println("^C")
			c.agent.Interrupt()
			currentInput = ""
			cursorPos = 0

		case 21: // Ctrl+U - Clear line
			currentInput = ""
			cursorPos = 0

		case 23: // Ctrl+W - Delete word
			// Find start of current word
			i := cursorPos - 1
			for i >= 0 && currentInput[i] == ' ' {
				i--
			}
			for i >= 0 && currentInput[i] != ' ' {
				i--
			}
			currentInput = currentInput[:i+1] + currentInput[cursorPos:]
			cursorPos = i + 1

		default:
			// Printable character
			if ch >= 32 && ch < 127 {
				currentInput = currentInput[:cursorPos] + string(ch) + currentInput[cursorPos:]
				cursorPos++
			}
		}
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
		fmt.Println("  /test         - Quick test the LLM connection")
		fmt.Println("  /clear        - Clear the screen")
		fmt.Println()
		fmt.Println("Keyboard shortcuts:")
		fmt.Println("  Up/Down       - Navigate command history")
		fmt.Println("  Left/Right    - Move cursor")
		fmt.Println("  Ctrl+U        - Clear line")
		fmt.Println("  Ctrl+W        - Delete word")
		fmt.Println("  Ctrl+C        - Interrupt current operation")
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
	case "/test":
		c.testLLM()
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

func (c *CLI) testLLM() {
	fmt.Println()
	fmt.Println("Testing LLM connection...")
	fmt.Printf("Model: %s\n", c.agent.GetModel())
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	start := time.Now()
	response, err := c.agent.Chat(ctx, "List the days of the week in order.")
	elapsed := time.Since(start)

	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		fmt.Println()
		fmt.Println("Troubleshooting:")
		fmt.Println("  - Check your internet connection")
		fmt.Println("  - Verify API credentials are set correctly")
		fmt.Println("  - For Bedrock: ensure AWS credentials are configured (aws sts get-caller-identity)")
		fmt.Println("  - For Bedrock: verify the model is enabled in your AWS region")
		return
	}

	fmt.Printf("OK (%.2fs)\n", elapsed.Seconds())
	fmt.Println()
	fmt.Println(response)
	fmt.Println()
}
