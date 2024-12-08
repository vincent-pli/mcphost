package cmd

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"
    
    "github.com/charmbracelet/log"

    "github.com/anthropics/anthropic-sdk-go"
    mcpclient "github.com/mark3labs/mcp-go/client"
    "github.com/mark3labs/mcp-go/mcp"
)

type MCPConfig struct {
    MCPServers map[string]struct {
        Command string   `json:"command"`
        Args    []string `json:"args"`
    } `json:"mcpServers"`
}

func mcpToolsToAnthropicTools(serverName string, mcpTools []mcp.Tool) []anthropic.ToolParam {
    anthropicTools := make([]anthropic.ToolParam, len(mcpTools))
    
    for i, tool := range mcpTools {
        namespacedName := fmt.Sprintf("%s__%s", serverName, tool.Name)
        
        schemaMap := map[string]interface{}{
            "type":       tool.InputSchema.Type,
            "properties": tool.InputSchema.Properties,
        }
        if len(tool.InputSchema.Required) > 0 {
            schemaMap["required"] = tool.InputSchema.Required
        }
        
        anthropicTools[i] = anthropic.ToolParam{
            Name:        anthropic.F(namespacedName),
            Description: anthropic.F(tool.Description),
            InputSchema: anthropic.Raw[interface{}](schemaMap),
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
        return nil, fmt.Errorf("error reading config file %s: %w", configPath, err)
    }

    var config MCPConfig
    if err := json.Unmarshal(configData, &config); err != nil {
        return nil, fmt.Errorf("error parsing config file: %w", err)
    }

    return &config, nil
}

func createMCPClients(config *MCPConfig) (map[string]*mcpclient.StdioMCPClient, error) {
    clients := make(map[string]*mcpclient.StdioMCPClient)
    
    for name, server := range config.MCPServers {
        client, err := mcpclient.NewStdioMCPClient(server.Command, server.Args...)
        if err != nil {
            for _, c := range clients {
                c.Close()
            }
            return nil, fmt.Errorf("failed to create MCP client for %s: %w", name, err)
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
            return nil, fmt.Errorf("failed to initialize MCP client for %s: %w", name, err)
        }

        clients[name] = client
    }
    
    return clients, nil
}
