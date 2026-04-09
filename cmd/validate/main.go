package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/aidanhall34/make-mcp/pkg/parser"
)

// stringSlice is a repeatable flag value that accumulates multiple --makefile arguments.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)

	var (
		configPath string
		delimiter  string
		makefiles  stringSlice
		strict     bool
	)

	fs.StringVar(&configPath, "config", "", "path to make-mcp.yml configuration file")
	fs.StringVar(&delimiter, "delimiter", "", "annotation delimiter (default: \"@\")")
	fs.Var(&makefiles, "makefile", "Makefile path to validate; may be repeated for multiple files")
	fs.BoolVar(&strict, "strict", false, "require every supported recipe annotation, including optional MCP tool hints")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	// Start from defaults, then layer the config file, then layer CLI flags.
	cfg := config.Default()

	if configPath != "" {
		fileCfg, err := config.LoadFile(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfg = config.Merge(cfg, fileCfg)
	}

	cliFlagsOverride := config.Config{
		Delimiter: delimiter,
		Makefiles: []string(makefiles),
		Strict:    strict,
	}
	cfg = config.Merge(cfg, cliFlagsOverride)

	if len(cfg.Makefiles) == 0 {
		return fmt.Errorf("no Makefiles specified; use --makefile <path> or set makefiles in make-mcp.yml")
	}

	opts := parser.ParseOptions{Delimiter: cfg.Delimiter, Strict: cfg.Strict}
	result, err := parser.ParseMakefiles(cfg.Makefiles, opts)
	if err != nil {
		return err
	}

	if len(result.Recipes) == 0 {
		fmt.Println("No annotated recipes found.")
		return nil
	}

	fmt.Printf("Found %d annotated recipe(s):\n\n", len(result.Recipes))
	for _, r := range result.Recipes {
		if r.SourceFile != "" {
			fmt.Printf("  [%s] %s  (%s)\n", r.ID, r.Name, r.SourceFile)
		} else {
			fmt.Printf("  [%s] %s\n", r.ID, r.Name)
		}
		fmt.Printf("    description:  %s\n", r.Description)
		fmt.Printf("    risk:         %s\n", r.Risk)
		if len(r.Params) == 0 {
			fmt.Printf("    params:       none\n")
		} else {
			for _, p := range r.Params {
				fmt.Printf("    param:        %s %s | %s\n", p.Name, p.Type, p.Description)
			}
		}
		fmt.Printf("    output:       %s\n", r.Output)
		fmt.Printf("    output-type:  %s\n", r.OutputType)
		fmt.Println()
	}

	if !result.Valid() {
		fmt.Fprintf(os.Stderr, "Validation failed with %d error(s):\n", len(result.Errors))
		for _, ve := range result.Errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", ve)
		}
		os.Exit(2)
	}

	fmt.Println("All recipes are valid.")
	return nil
}
