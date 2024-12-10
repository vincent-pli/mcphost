package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"

	"strings"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

var (
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
			AlignHorizontal(lipgloss.Left)

	toolNameStyle = lipgloss.NewStyle().
			Foreground(tokyoCyan).
			Bold(true)

	descriptionStyle = lipgloss.NewStyle().
				Foreground(tokyoFg).
				PaddingBottom(1)
)

type MCPConfig struct {
	MCPServers map[string]struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	} `json:"mcpServers"`
}

func mcpToolsToAnthropicTools(
	serverName string,
	mcpTools []mcp.Tool,
) []Tool {
	anthropicTools := make([]Tool, len(mcpTools))

	for i, tool := range mcpTools {
		namespacedName := fmt.Sprintf("%s__%s", serverName, tool.Name)

		anthropicTools[i] = Tool{
			Name:        namespacedName,
			Description: tool.Description,
			InputSchema: InputSchema{
				Type:       tool.InputSchema.Type,
				Properties: tool.InputSchema.Properties,
				Required:   tool.InputSchema.Required,
			},
		}
	}

	return anthropicTools
}

func loadMCPConfig() (*MCPConfig, error) {
	var configPath string
	if configFile != "" {
		configPath = configFile
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("error getting home directory: %w", err)
		}
		configPath = filepath.Join(homeDir, ".mcp.json")
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		defaultConfig := MCPConfig{
			MCPServers: make(map[string]struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			}),
		}

		// Create the file with default config
		configData, err := json.MarshalIndent(defaultConfig, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("error creating default config: %w", err)
		}

		if err := os.WriteFile(configPath, configData, 0644); err != nil {
			return nil, fmt.Errorf("error writing default config file: %w", err)
		}

		log.Info("Created default config file", "path", configPath)
		return &defaultConfig, nil
	}

	// Read existing config
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf(
			"error reading config file %s: %w",
			configPath,
			err,
		)
	}

	var config MCPConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	return &config, nil
}

func createMCPClients(
	config *MCPConfig,
) (map[string]*mcpclient.StdioMCPClient, error) {
	clients := make(map[string]*mcpclient.StdioMCPClient)

	for name, server := range config.MCPServers {
		client, err := mcpclient.NewStdioMCPClient(
			server.Command,
			server.Args...)
		if err != nil {
			for _, c := range clients {
				c.Close()
			}
			return nil, fmt.Errorf(
				"failed to create MCP client for %s: %w",
				name,
				err,
			)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		log.Info("Initializing server...", "name", name)
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "mcphost",
			Version: "0.1.0",
		}

		_, err = client.Initialize(ctx, initRequest)
		if err != nil {
			client.Close()
			for _, c := range clients {
				c.Close()
			}
			return nil, fmt.Errorf(
				"failed to initialize MCP client for %s: %w",
				name,
				err,
			)
		}

		clients[name] = client
	}

	return clients, nil
}

func handleSlashCommand(
	prompt string,
	mcpConfig *MCPConfig,
	mcpClients map[string]*mcpclient.StdioMCPClient,
) (bool, error) {
	if !strings.HasPrefix(prompt, "/") {
		return false, nil
	}

	switch strings.ToLower(strings.TrimSpace(prompt)) {
	case "/tools":
		handleToolsCommand(mcpClients)
		return true, nil
	case "/help":
		handleHelpCommand()
		return true, nil
	case "/servers":
		handleServersCommand(mcpConfig)
		return true, nil
	case "/quit":
		fmt.Println("\nGoodbye!")
		defer os.Exit(0)
		return true, nil
	default:
		fmt.Printf("%s\nType /help to see available commands\n\n",
			errorStyle.Render("Unknown command: "+prompt))
		return true, nil
	}
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

	// If tools are disabled (empty client map), show a message
	if len(mcpClients) == 0 {
		termWidth := getTerminalWidth()
		contentWidth := termWidth - 20
		serverBoxStyle := serverBox.Width(contentWidth)

		message := "Tools are currently disabled for this model."
		boxedContent := serverBoxStyle.Render(message)
		fmt.Print(boxedContent)
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
			defer cancel()

			toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
			if err != nil {
				results[serverName] = serverTools{
					tools: nil,
					err:   err,
				}
				continue
			}

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
		fmt.Print(boxedContent)
	}
}
