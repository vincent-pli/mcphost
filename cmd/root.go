package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/log"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	renderer *glamour.TermRenderer

	// Tokyo Night theme colors
	tokyoPurple = lipgloss.Color("99")  // #9d7cd8
	tokyoCyan   = lipgloss.Color("73")  // #7dcfff
	tokyoBlue   = lipgloss.Color("111") // #7aa2f7
	tokyoGreen  = lipgloss.Color("120") // #73daca
	tokyoRed    = lipgloss.Color("203") // #f7768e
	tokyoOrange = lipgloss.Color("215") // #ff9e64
	tokyoFg     = lipgloss.Color("189") // #c0caf5
	tokyoGray   = lipgloss.Color("237") // #3b4261
	tokyoBg     = lipgloss.Color("234") // #1a1b26

	serverCommandStyle = lipgloss.NewStyle().
				Foreground(tokyoOrange).
				Bold(true)

	serverArgumentsStyle = lipgloss.NewStyle().
				Foreground(tokyoFg)

	serverHeaderStyle = lipgloss.NewStyle().
				Foreground(tokyoCyan).
				Bold(true)

	promptStyle = lipgloss.NewStyle().
			Foreground(tokyoBlue).
			PaddingLeft(2)

	responseStyle = lipgloss.NewStyle().
			Foreground(tokyoFg).
			PaddingLeft(2)

	errorStyle = lipgloss.NewStyle().
			Foreground(tokyoRed).
			Bold(true)

	serverBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tokyoPurple).
			Padding(1).
			MarginBottom(1).
			AlignHorizontal(lipgloss.Left) // Force left alignment

	toolNameStyle = lipgloss.NewStyle().
			Foreground(tokyoCyan).
			Bold(true)

	descriptionStyle = lipgloss.NewStyle().
				Foreground(tokyoFg).
				PaddingBottom(1)

	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "mcphost",
	Short: "Chat with Claude 3.5 Sonnet or Ollama models",
	Long: `MCPHost is a CLI tool that allows you to interact with Claude 3.5 Sonnet or Ollama models.
It supports various tools through MCP servers and provides streaming responses.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMCPHost()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().
		StringVar(&configFile, "config", "", "config file (default is $HOME/mcp.json)")
}

func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 80 // Fallback width
	}
	return width - 20
}

func updateRenderer() error {
	width := getTerminalWidth()
	var err error
	renderer, err = glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	return err
}

func handleHelpCommand() {
	if err := updateRenderer(); err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error updating renderer: %v", err)),
		)
		return
	}
	var markdown strings.Builder

	markdown.WriteString("# Available Commands\n\n")
	markdown.WriteString("The following commands are available:\n\n")
	markdown.WriteString("- **/help**: Show this help message\n")
	markdown.WriteString("- **/tools**: List all available tools\n")
	markdown.WriteString("- **/servers**: List configured MCP servers\n")
	markdown.WriteString("- **/quit**: Exit the application\n")
	markdown.WriteString("\nYou can also press Ctrl+C at any time to quit.\n")
	markdown.WriteString("\n## Subcommands\n\n")
	markdown.WriteString(
		"- **ollama**: Use an Ollama model instead of Claude\n",
	)
	markdown.WriteString("  Example: `mcphost ollama --model mistral`\n")

	rendered, err := renderer.Render(markdown.String())
	if err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error rendering help: %v", err)),
		)
		return
	}

	fmt.Print(rendered)
}

func handleServersCommand(config *MCPConfig) {
	if err := updateRenderer(); err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error updating renderer: %v", err)),
		)
		return
	}

	var markdown strings.Builder
	action := func() {
		if len(config.MCPServers) == 0 {
			markdown.WriteString("No servers configured.\n")
		} else {
			for name, server := range config.MCPServers {
				markdown.WriteString(fmt.Sprintf("# %s\n\n", name))
				markdown.WriteString("*Command*\n")
				markdown.WriteString(fmt.Sprintf("`%s`\n\n", server.Command))

				markdown.WriteString("*Arguments*\n")
				if len(server.Args) > 0 {
					markdown.WriteString(fmt.Sprintf("`%s`\n", strings.Join(server.Args, " ")))
				} else {
					markdown.WriteString("*None*\n")
				}
			}
		}
	}

	_ = spinner.New().
		Title("Loading server configuration...").
		Action(action).
		Run()
	rendered, err := renderer.Render(markdown.String())
	if err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error rendering servers: %v", err)),
		)
		return
	}

	// Calculate width with proper margins
	termWidth := getTerminalWidth()
	contentWidth := termWidth - 20 // Reserve space for margins

	// Create a box style with the calculated width
	boxStyle := serverBox.Width(contentWidth)

	// Wrap the rendered markdown in the box
	boxedContent := boxStyle.Render(rendered)
	fmt.Print(boxedContent)
}

func handleToolsCommand(mcpClients map[string]*mcpclient.StdioMCPClient) {
	if err := updateRenderer(); err != nil {
		fmt.Printf(
			"\n%s\n",
			errorStyle.Render(fmt.Sprintf("Error updating renderer: %v", err)),
		)
		return
	}

	// Calculate widths with proper margins
	termWidth := getTerminalWidth()
	contentWidth := termWidth - 20 // Reserve space for margins

	// Update styles with calculated widths
	serverBoxStyle := serverBox.Width(contentWidth)

	type serverTools struct {
		tools []mcp.Tool
		err   error
	}
	results := make(map[string]serverTools)

	action := func() {
		for serverName, mcpClient := range mcpClients {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel() // Move defer here to ensure it's called

			toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
			if err != nil {
				results[serverName] = serverTools{
					tools: nil,
					err:   err,
				}
				continue
			}

			// Only access Tools if toolsResult is not nil
			var tools []mcp.Tool
			if toolsResult != nil {
				tools = toolsResult.Tools
			}

			results[serverName] = serverTools{
				tools: tools,
				err:   nil,
			}
		}
	}
	_ = spinner.New().
		Title("Fetching tools from all servers...").
		Action(action).
		Run()

	// Now display all results
	for serverName, result := range results {
		if result.err != nil {
			errMsg := errorStyle.Render(
				fmt.Sprintf(
					"Error fetching tools from %s: %v",
					serverName,
					result.err,
				),
			)
			fmt.Printf("\n%s\n", serverBox.Render(errMsg))
			continue
		}

		serverHeader := fmt.Sprintf("# %s\n", serverName)
		renderedHeader, err := renderer.Render(serverHeader)
		if err != nil {
			errMsg := errorStyle.Render(
				fmt.Sprintf("Error rendering server header: %v", err),
			)
			fmt.Printf("\n%s\n", serverBox.Render(errMsg))
			continue
		}

		var content strings.Builder
		content.WriteString(renderedHeader)

		if len(result.tools) == 0 {
			content.WriteString("\nNo tools available.\n")
		} else {
			for _, tool := range result.tools {
				toolDisplay := fmt.Sprintf("%s\n%s",
					toolNameStyle.Render("🔧 "+tool.Name),
					descriptionStyle.Render(tool.Description),
				)
				content.WriteString(toolDisplay + "\n")
			}
		}

		boxedContent := serverBoxStyle.Render(content.String())
		// Print directly without the centeredBox wrapper
		fmt.Print(boxedContent)
	}
}

func runPrompt(
	client *anthropic.Client,
	mcpClients map[string]*mcpclient.StdioMCPClient,
	tools []anthropic.ToolParam,
	prompt string,
	messages *[]anthropic.MessageParam,
) error {
	// Display the user's prompt if it's not empty (i.e., not a tool response)
	if prompt != "" {
		fmt.Printf("\n%s\n", promptStyle.Render("You: "+prompt))
		*messages = append(
			*messages,
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		)
	}

	var messagePtr *anthropic.Message
	var err error
	action := func() {
		messagePtr, err = client.Messages.New(
			context.Background(),
			anthropic.MessageNewParams{
				Model: anthropic.F(
					anthropic.ModelClaude_3_5_Sonnet_20240620,
				),
				MaxTokens: anthropic.F(int64(4096)),
				Messages:  anthropic.F(*messages),
				Tools:     anthropic.F(tools),
			},
		)
	}
	_ = spinner.New().Title("Thinking...").Action(action).Run()

	if err != nil {
		return err
	}

	message := *messagePtr // Dereference the pointer
	fmt.Print(responseStyle.Render("\nClaude: "))

	toolResults := []anthropic.ContentBlockParamUnion{}

	for _, block := range message.Content {
		switch block := block.AsUnion().(type) {
		case anthropic.TextBlock:
			if err := updateRenderer(); err != nil {
				return fmt.Errorf("error updating renderer: %v", err)
			}
			str, err := renderer.Render(block.Text + "\n")
			if err != nil {
				log.Error("Failed to render response", "error", err)
				fmt.Print(block.Text + "\n")
				continue
			}
			fmt.Print(str)

		case anthropic.ToolUseBlock:
			log.Info("🔧 Using tool", "name", block.Name)

			parts := strings.Split(block.Name, "__")
			if len(parts) != 2 {
				fmt.Printf("Error: Invalid tool name format: %s\n", block.Name)
				continue
			}

			serverName, toolName := parts[0], parts[1]
			mcpClient, ok := mcpClients[serverName]
			if !ok {
				fmt.Printf("Error: Server not found: %s\n", serverName)
				continue
			}

			var toolArgs map[string]interface{}
			if err := json.Unmarshal(block.Input, &toolArgs); err != nil {
				fmt.Printf("Error parsing tool arguments: %v\n", err)
				continue
			}

			var toolResultPtr *mcp.CallToolResult
			action := func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				toolResultPtr, err = mcpClient.CallTool(ctx, mcp.CallToolRequest{
					Params: struct {
						Name      string                 `json:"name"`
						Arguments map[string]interface{} `json:"arguments,omitempty"`
					}{
						Name:      toolName,
						Arguments: toolArgs,
					},
				})
			}
			_ = spinner.New().
				Title(fmt.Sprintf("Running tool %s...", toolName)).
				Action(action).
				Run()

			if err != nil {
				errMsg := fmt.Sprintf("Error calling tool %s: %v", toolName, err)
				fmt.Printf("\n%s\n", errorStyle.Render(errMsg))

				// Add error message as tool result
				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(block.ID, errMsg, true))
				continue
			}

			toolResult := *toolResultPtr // Dereference the pointer
			resultJSON, err := json.Marshal(toolResult)
			if err != nil {
				fmt.Printf("Error marshaling tool result: %v\n", err)
				continue
			}

			toolResults = append(toolResults,
				anthropic.NewToolResultBlock(block.ID, string(resultJSON), toolResult.IsError))
		}
	}

	*messages = append(*messages, message.ToParam())

	if len(toolResults) > 0 {
		*messages = append(*messages, anthropic.NewUserMessage(toolResults...))
		// Make another call to get Claude's response to the tool results
		return runPrompt(client, mcpClients, tools, "", messages)
	}

	fmt.Println() // Add spacing
	return nil
}

func runMCPHost() error {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}

	mcpConfig, err := loadMCPConfig()
	if err != nil {
		return fmt.Errorf("error loading MCP config: %v", err)
	}

	mcpClients, err := createMCPClients(mcpConfig)
	if err != nil {
		return fmt.Errorf("error creating MCP clients: %v", err)
	}

	defer func() {
		log.Info("Shutting down MCP servers...")
		for name, client := range mcpClients {
			if err := client.Close(); err != nil {
				log.Error("Failed to close server", "name", name, "error", err)
			} else {
				log.Info("Server closed", "name", name)
			}
		}
	}()

	for name := range mcpClients {
		log.Info("Server connected", "name", name)
	}

	client := anthropic.NewClient(
		option.WithAPIKey(apiKey),
	)

	var allTools []anthropic.ToolParam
	for serverName, mcpClient := range mcpClients {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
		cancel()

		if err != nil {
			log.Error(
				"Error fetching tools",
				"server",
				serverName,
				"error",
				err,
			)
			continue
		}

		serverTools := mcpToolsToAnthropicTools(serverName, toolsResult.Tools)
		allTools = append(allTools, serverTools...)
		log.Info(
			"Tools loaded",
			"server",
			serverName,
			"count",
			len(toolsResult.Tools),
		)
	}

	if err := updateRenderer(); err != nil {
		return fmt.Errorf("error initializing renderer: %v", err)
	}

	messages := make([]anthropic.MessageParam, 0)

	// Main interaction loop
	for {
		width := getTerminalWidth()
		var prompt string
		form := huh.NewForm(
			huh.NewGroup(
				huh.NewText().
					Key("prompt").
					Title("Enter your prompt (Type /help for commands, Ctrl+C to quit)").
					Value(&prompt),
			),
		).WithWidth(width)

		err := form.Run()
		if err != nil {
			// Check if it's a user abort (Ctrl+C)
			if err.Error() == "user aborted" {
				fmt.Println("\nGoodbye!")
				return nil // Exit cleanly
			}
			return err // Return other errors normally
		}

		prompt = form.GetString("prompt")
		if prompt == "" {
			continue
		}

		// Handle slash commands
		if strings.HasPrefix(prompt, "/") {
			switch strings.ToLower(strings.TrimSpace(prompt)) {
			case "/tools":
				handleToolsCommand(mcpClients)
				continue
			case "/help":
				handleHelpCommand()
				continue
			case "/servers":
				handleServersCommand(mcpConfig)
				continue
			case "/quit":
				fmt.Println("\nGoodbye!")
				return nil
			default:
				fmt.Printf("%s\nType /help to see available commands\n\n",
					errorStyle.Render("Unknown command: "+prompt))
				continue
			}
		}

		err = runPrompt(client, mcpClients, allTools, prompt, &messages)
		if err != nil {
			return err
		}
	}
}
