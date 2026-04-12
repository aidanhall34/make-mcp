package server

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func jsonRPCError(code int, message string, data any) error {
	details := mcp.NewJSONRPCErrorDetails(code, message, data)
	return details.AsError()
}
