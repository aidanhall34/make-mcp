package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/parser"
)

const (
	// ErrorCodeInvalidParams reports a request that does not match the recipe schema.
	ErrorCodeInvalidParams = "invalid_params"

	// ErrorCodeTimeout reports that make exceeded the configured timeout.
	ErrorCodeTimeout = "tool_timeout"

	// ErrorCodeExitNonZero reports that make exited with a non-zero exit code.
	ErrorCodeExitNonZero = "tool_exit_nonzero"
)

// Request describes a single make invocation.
type Request struct {
	Recipe   parser.Recipe
	Args     map[string]any
	Timeout  time.Duration
	MakePath string
	// RootDir, when non-empty, sets the working directory for make and suppresses
	// the -f flag. Use this so sub-makefiles inherit variables from the root
	// makefile (which includes them all) rather than being invoked standalone.
	RootDir string
}

// Result contains the captured process output.
type Result struct {
	Stdout string
	Stderr string
}

// Error describes a structured runner failure.
type Error struct {
	Code     string
	Message  string
	ExitCode *int
	Stdout   string
	Stderr   string
	TimedOut bool
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Run executes make for a single parsed recipe.
func Run(ctx context.Context, req Request) (Result, error) {
	return RunStream(ctx, req, nil, nil)
}

// RunStream executes make for a single parsed recipe, optionally streaming
// output to the provided writers.
func RunStream(ctx context.Context, req Request, stdout, stderr io.Writer) (Result, error) {
	if req.Recipe.ID == "" {
		return Result{}, &Error{
			Code:    ErrorCodeInvalidParams,
			Message: "recipe is missing target ID",
		}
	}

	makePath := req.MakePath
	if makePath == "" {
		makePath = "make"
	}

	sourceMakefile := req.Recipe.SourceFile
	if req.RootDir != "" {
		sourceMakefile = ""
	}
	args, err := buildArgs(req.Recipe, req.Args, sourceMakefile)
	if err != nil {
		return Result{}, err
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, makePath, args...)
	if req.RootDir != "" {
		cmd.Dir = req.RootDir
	} else {
		cmd.Dir = workingDir(req.Recipe.SourceFile)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	if stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdoutBuf, stdout)
	} else {
		cmd.Stdout = &stdoutBuf
	}
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderrBuf, stderr)
	} else {
		cmd.Stderr = &stderrBuf
	}

	err = cmd.Run()
	result := Result{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}
	if err == nil {
		return result, nil
	}

	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return Result{}, &Error{
			Code:     ErrorCodeTimeout,
			Message:  fmt.Sprintf("make target exceeded timeout of %s", timeout),
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			TimedOut: true,
		}
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode := exitErr.ExitCode()
		return Result{}, &Error{
			Code:     ErrorCodeExitNonZero,
			Message:  fmt.Sprintf("make target failed with exit code %d", exitCode),
			ExitCode: &exitCode,
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
		}
	}

	return Result{}, fmt.Errorf("run make: %w", err)
}

func workingDir(sourceFile string) string {
	if sourceFile == "" {
		return ""
	}
	return filepath.Dir(sourceFile)
}

func buildArgs(recipe parser.Recipe, provided map[string]any, sourceMakefile string) ([]string, error) {
	allowed := map[string]parser.Param{}
	for _, param := range recipe.Params {
		allowed[param.Name] = param
	}

	args := []string{}
	args = append(args, "--no-print-directory")
	if sourceMakefile != "" {
		args = append(args, "-f", filepath.Base(sourceMakefile))
	}
	args = append(args, recipe.ID)

	for key := range provided {
		if _, ok := allowed[key]; !ok {
			return nil, &Error{
				Code:    ErrorCodeInvalidParams,
				Message: fmt.Sprintf("unknown param %q for tool %q", key, recipe.ID),
			}
		}
	}

	for _, param := range recipe.Params {
		value, ok := provided[param.Name]
		if !ok {
			continue
		}
		text, err := stringifyValue(param, value)
		if err != nil {
			return nil, err
		}
		args = append(args, makeVariableName(param.Name)+"="+text)
	}

	return args, nil
}

func stringifyValue(param parser.Param, value any) (string, error) {
	switch param.Type {
	case parser.ParamTypeString:
		text, ok := value.(string)
		if !ok {
			return "", invalidParamTypeError(param.Name, "string", value)
		}
		return text, nil
	case parser.ParamTypeInt:
		switch v := value.(type) {
		case int:
			return strconv.Itoa(v), nil
		case int8, int16, int32, int64:
			return fmt.Sprintf("%d", v), nil
		case uint, uint8, uint16, uint32, uint64:
			return fmt.Sprintf("%d", v), nil
		case float64:
			if v != float64(int64(v)) {
				return "", invalidParamTypeError(param.Name, "integer", value)
			}
			return strconv.FormatInt(int64(v), 10), nil
		case float32:
			if v != float32(int64(v)) {
				return "", invalidParamTypeError(param.Name, "integer", value)
			}
			return strconv.FormatInt(int64(v), 10), nil
		default:
			return "", invalidParamTypeError(param.Name, "integer", value)
		}
	case parser.ParamTypeBool:
		b, ok := value.(bool)
		if !ok {
			return "", invalidParamTypeError(param.Name, "boolean", value)
		}
		return strconv.FormatBool(b), nil
	default:
		return "", &Error{
			Code:    ErrorCodeInvalidParams,
			Message: fmt.Sprintf("unsupported param type %q for %q", param.Type, param.Name),
		}
	}
}

func invalidParamTypeError(name, want string, got any) error {
	return &Error{
		Code:    ErrorCodeInvalidParams,
		Message: fmt.Sprintf("param %q must be %s; got %T", name, want, got),
	}
}

func makeVariableName(name string) string {
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToUpper(name)
}
