package transport

import (
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// ServeStdio starts the stdio MCP transport.
func ServeStdio(server *mcpserver.MCPServer) error {
	return mcpserver.ServeStdio(server)
}
