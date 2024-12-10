# MCPHost 🤖

A CLI host application that enables Large Language Models (LLMs) to interact with external tools through the Model Context Protocol (MCP). Currently supports both Claude 3.5 Sonnet and Ollama models.

## Overview 🌟

MCPHost acts as a host in the MCP client-server architecture, where:
- **Hosts** (like MCPHost) are LLM applications that manage connections and interactions
- **Clients** maintain 1:1 connections with MCP servers
- **Servers** provide context, tools, and capabilities to the LLMs

This architecture allows language models to:
- Access external tools and data sources 🛠️
- Maintain consistent context across interactions 🔄
- Execute commands and retrieve information safely 🔒

Currently supports:
- Claude 3.5 Sonnet (claude-3-5-sonnet-20240620)
- Any Ollama-compatible model with function calling support

## Features ✨

- Interactive conversations with either Claude 3.5 Sonnet or Ollama models
- Support for multiple concurrent MCP servers
- Dynamic tool discovery and integration
- Tool calling capabilities for both model types
- Configurable MCP server locations and arguments
- Consistent command interface across model types
- Configurable message history window for context management

## Requirements 📋

- Go 1.23.3 or later
- For Claude: An Anthropic API key
- For Ollama: Local Ollama installation with desired models
- One or more MCP-compatible tool servers

## Environment Setup 🔧

1. Anthropic API Key (for Claude):
```bash
export ANTHROPIC_API_KEY='your-api-key'
```

2. Ollama Setup:
- Install Ollama from https://ollama.ai
- Pull your desired model:
```bash
ollama pull mistral
```
- Ensure Ollama is running:
```bash
ollama serve
```

## Installation 📦

```bash
go install github.com/mark3labs/mcphost@latest
```

## Configuration ⚙️

MCPHost will automatically create a configuration file at `~/.mcp.json` if it doesn't exist. You can also specify a custom location using the `--config` flag:

```json
{
  "mcpServers": {
    "sqlite": {
      "command": "uvx",
      "args": [
        "mcp-server-sqlite",
        "--db-path",
        "/tmp/foo.db"
      ]
    },
    "filesystem": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/tmp"
      ]
    }
  }
}
```

Each MCP server entry requires:
- `command`: The command to run (e.g., `uvx`, `npx`) 
- `args`: Array of arguments for the command:
  - For SQLite server: `mcp-server-sqlite` with database path
  - For filesystem server: `@modelcontextprotocol/server-filesystem` with directory path

## Usage 🚀

### Basic Commands

1. Using Claude 3.5 Sonnet:
```bash
mcphost
```

2. Using Ollama:
```bash
mcphost ollama --model mistral
```
Note: Tool support in Ollama requires models that support function calling.

### Additional Options

- Custom config file:
```bash
mcphost --config /path/to/config.json
```

- Set message history window:
```bash
mcphost --message-window 15
```

### Interactive Commands

While chatting, you can use:
- `/help`: Show available commands
- `/tools`: List all available tools
- `/servers`: List configured MCP servers
- `/history`: Display conversation history
- `/quit`: Exit the application
- `Ctrl+C`: Exit at any time

### Global Flags
- `--config`: Specify custom config file location
- `--message-window`: Set number of messages to keep in context (default: 10)

## MCP Server Compatibility 🔌

MCPHost can work with any MCP-compliant server. For examples and reference implementations, see the [MCP Servers Repository](https://github.com/modelcontextprotocol/servers).

## Contributing 🤝

Contributions are welcome! Feel free to:
- Submit bug reports or feature requests through issues
- Create pull requests for improvements
- Share your custom MCP servers
- Improve documentation

Please ensure your contributions follow good coding practices and include appropriate tests.

## License 📄

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments 🙏

- Thanks to the Anthropic team for Claude and the MCP specification
- Thanks to the Ollama team for their local LLM runtime
- Thanks to all contributors who have helped improve this tool
