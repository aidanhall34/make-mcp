package server

import (
	"github.com/aidanhall34/make-mcp/pkg/runner"
	"github.com/mark3labs/mcp-go/mcp"
)

func runnerErrorToJSONRPC(err *runner.Error) error {
	data := map[string]any{
		"stdout":    err.Stdout,
		"stderr":    err.Stderr,
		"timed_out": err.TimedOut,
	}
	if err.ExitCode != nil {
		data["exit_code"] = *err.ExitCode
	}
	return jsonRPCError(-32000, err.Message, data)
}

func jsonRPCError(code int, message string, data any) error {
	details := mcp.NewJSONRPCErrorDetails(code, message, data)
	return details.AsError()
}
