package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/log"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	api "github.com/ollama/ollama/api"
	"github.com/spf13/cobra"
)

// F is a helper function to get a pointer to a value
func F[T any](v T) *T {
	return &v
}

var (
	modelName string
	ollamaCmd = &cobra.Command{
		Use:   "ollama",
		Short: "Chat using an Ollama model",
		Long:  `Use a local Ollama model for chat with MCP tool support`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOllama()
		},
	}
)

func modelSupportsTools(client *api.Client, modelName string) bool {
	resp, err := client.Show(context.Background(), &api.ShowRequest{
		Model: modelName,
	})

	if err != nil {
		log.Error("Failed to get model info", "error", err)
		return false
	}

	// Check if model details indicate function calling support
	// This looks for function calling capability in model details
	if resp.Modelfile != "" {
		if strings.Contains(resp.Modelfile, "<tools>") {
			return true
		}
	}

	return false
}

func init() {
	ollamaCmd.Flags().
		StringVar(&modelName, "model", "", "Ollama model to use (required)")
	_ = ollamaCmd.MarkFlagRequired("model")
	rootCmd.AddCommand(ollamaCmd)
}

func mcpToolsToOllamaTools(serverName string, mcpTools []mcp.Tool) []api.Tool {
	ollamaTools := make([]api.Tool, len(mcpTools))

	for i, tool := range mcpTools {
		namespacedName := fmt.Sprintf("%s__%s", serverName, tool.Name)

		ollamaTools[i] = api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        namespacedName,
				Description: tool.Description,
				Parameters: struct {
					Type       string   `json:"type"`
					Required   []string `json:"required"`
					Properties map[string]struct {
						Type        string   `json:"type"`
						Description string   `json:"description"`
						Enum        []string `json:"enum,omitempty"`
					} `json:"properties"`
				}{
					Type:     tool.InputSchema.Type,
					Required: tool.InputSchema.Required,
					Properties: make(map[string]struct {
						Type        string   `json:"type"`
						Description string   `json:"description"`
						Enum        []string `json:"enum,omitempty"`
					}),
				},
			},
		}

		// Convert properties
		for propName, prop := range tool.InputSchema.Properties {
			propMap, ok := prop.(map[string]interface{})
			if !ok {
				log.Error("Invalid property type", "property", propName)
				continue
			}

			propType, _ := propMap["type"].(string)
			propDesc, _ := propMap["description"].(string)
			propEnumRaw, hasEnum := propMap["enum"]

			var enumVals []string
			if hasEnum {
				if enumSlice, ok := propEnumRaw.([]interface{}); ok {
					enumVals = make([]string, len(enumSlice))
					for i, v := range enumSlice {
						if str, ok := v.(string); ok {
							enumVals[i] = str
						}
					}
				}
			}

			ollamaTools[i].Function.Parameters.Properties[propName] = struct {
				Type        string   `json:"type"`
				Description string   `json:"description"`
				Enum        []string `json:"enum,omitempty"`
			}{
				Type:        propType,
				Description: propDesc,
				Enum:        enumVals,
			}
		}
	}

	return ollamaTools
}

func runOllama() error {
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

	client, err := api.ClientFromEnvironment()
	if err != nil {
		return fmt.Errorf("error creating Ollama client: %v", err)
	}

	var allTools []api.Tool
	var activeClients map[string]*mcpclient.StdioMCPClient

	if modelSupportsTools(client, modelName) {
		activeClients = mcpClients // Use the full set of clients
		for serverName, mcpClient := range mcpClients {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
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

			serverTools := mcpToolsToOllamaTools(serverName, toolsResult.Tools)
			allTools = append(allTools, serverTools...)
			log.Info(
				"Tools loaded",
				"server",
				serverName,
				"count",
				len(toolsResult.Tools),
			)
		}
	} else {
		activeClients = nil // No active clients when tools are disabled
		fmt.Printf("\n%s\n\n",
			errorStyle.Render(fmt.Sprintf(
				"Warning: Model %s does not support function calling. Tools will be disabled.",
				modelName,
			)),
		)
	}

	if err := updateRenderer(); err != nil {
		return fmt.Errorf("error initializing renderer: %v", err)
	}

	// Initialize messages with system prompt
	messages := []api.Message{
		{
			Role: "system",
			Content: `You are a helpful AI assistant with access to external tools. Respond directly to questions and requests.
Only use tools when specifically needed to accomplish a task. If you can answer without using tools, do so.
When you do need to use a tool, explain what you're doing first.`,
		},
	}

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
			if err.Error() == "user aborted" {
				fmt.Println("\nGoodbye!")
				return nil
			}
			return err
		}

		prompt = form.GetString("prompt")
		if prompt == "" {
			continue
		}

		// Handle slash commands
		handled, err := handleSlashCommand(prompt, mcpConfig, activeClients)
		if err != nil {
			return err
		}
		if handled {
			continue
		}

		err = runOllamaPrompt(client, mcpClients, allTools, prompt, &messages)
		if err != nil {
			return err
		}
	}
}

func runOllamaPrompt(
	client *api.Client,
	mcpClients map[string]*mcpclient.StdioMCPClient,
	tools []api.Tool,
	prompt string,
	messages *[]api.Message,
) error {
	if prompt != "" {
		fmt.Printf("\n%s\n", promptStyle.Render("You: "+prompt))
		*messages = append(*messages, api.Message{
			Role:    "user",
			Content: prompt,
		})
	}

	var err error
	var responseContent string
	var toolCalls []api.ToolCall

	action := func() {
		err = client.Chat(context.Background(), &api.ChatRequest{
			Model:    modelName,
			Messages: *messages,
			Tools:    tools,
			Stream:   F(false), // Disable streaming
		}, func(response api.ChatResponse) error {
			if response.Done {
				responseContent = response.Message.Content
				toolCalls = response.Message.ToolCalls
				if len(toolCalls) > 0 && responseContent == "" {
					responseContent = "Using tools..."
				}
				*messages = append(*messages, response.Message)
			}
			return nil
		})
	}

	_ = spinner.New().Title("Thinking...").Action(action).Run()
	if err != nil {
		return err
	}

	// Print the response
	if err := updateRenderer(); err != nil {
		return fmt.Errorf("error updating renderer: %v", err)
	}

	fmt.Print(responseStyle.Render("\nAssistant: "))
	rendered, err := renderer.Render(responseContent + "\n")
	if err != nil {
		log.Error("Failed to render response", "error", err)
		fmt.Print(responseContent + "\n")
	} else {
		fmt.Print(rendered)
	}

	// Handle tool calls if present
	if len(toolCalls) > 0 {
		for _, toolCall := range toolCalls {
			log.Info(
				"🔧 Using tool",
				"name",
				toolCall.Function.Name,
			)

			parts := strings.Split(toolCall.Function.Name, "__")
			if len(parts) != 2 {
				fmt.Printf(
					"Error: Invalid tool name format: %s\n",
					toolCall.Function.Name,
				)
				continue
			}

			serverName, toolName := parts[0], parts[1]
			mcpClient, ok := mcpClients[serverName]
			if !ok {
				fmt.Printf("Error: Server not found: %s\n", serverName)
				continue
			}

			var toolResultPtr *mcp.CallToolResult
			action := func() {
				ctx, cancel := context.WithTimeout(
					context.Background(),
					10*time.Second,
				)
				defer cancel()

				toolResultPtr, err = mcpClient.CallTool(
					ctx,
					mcp.CallToolRequest{
						Params: struct {
							Name      string                 `json:"name"`
							Arguments map[string]interface{} `json:"arguments,omitempty"`
						}{
							Name:      toolName,
							Arguments: toolCall.Function.Arguments,
						},
					},
				)
			}
			_ = spinner.New().
				Title(fmt.Sprintf("Running tool %s...", toolName)).
				Action(action).
				Run()

			if err != nil {
				errMsg := fmt.Sprintf(
					"Error calling tool %s: %v",
					toolName,
					err,
				)
				fmt.Printf("\n%s\n", errorStyle.Render(errMsg))

				// Add error message directly to messages array as JSON string
				*messages = append(*messages, api.Message{
					Role:    "tool",
					Content: fmt.Sprintf(`{"error": "%s"}`, errMsg),
				})
				continue
			}

			toolResult := *toolResultPtr
			// Check if there's an error in the tool result
			if toolResult.IsError {
				errMsg := fmt.Sprintf("Tool error: %v", toolResult.Result)
				fmt.Printf("\n%s\n", errorStyle.Render(errMsg))
				*messages = append(*messages, api.Message{
					Role:    "tool",
					Content: fmt.Sprintf(`{"error": %q}`, errMsg),
				})
				continue
			}

			// Add the tool result directly to messages array as JSON string
			resultJSON, err := json.Marshal(toolResult.Content)
			if err != nil {
				errMsg := fmt.Sprintf("Error marshaling tool result: %v", err)
				fmt.Printf("\n%s\n", errorStyle.Render(errMsg))
				continue
			}

			*messages = append(*messages, api.Message{
				Role:    "tool",
				Content: string(resultJSON),
			})

		}

		// Make another call to get Ollama's response to the tool results
		return runOllamaPrompt(client, mcpClients, tools, "", messages)
	}

	fmt.Println() // Add spacing
	return nil
}
