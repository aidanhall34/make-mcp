package parser

import (
	"errors"
	"fmt"
	"mime"
)

// RiskLevel represents the potential impact of running a make recipe.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// ParamType is the expected value type of a recipe input parameter.
type ParamType string

const (
	ParamTypeString ParamType = "string"
	ParamTypeInt    ParamType = "int"
	ParamTypeBool   ParamType = "bool"
)

// Param describes a single named input parameter for a recipe.
// Parameters map to environment variables or Make variables read by the recipe.
type Param struct {
	// Name is the parameter identifier, e.g. "name" or "count".
	// It maps to the Make/environment variable the recipe reads.
	Name string

	// Type is the expected value type: string, int, or bool.
	Type ParamType

	// Description explains the parameter's purpose, valid values, and any defaults.
	Description string
}

// ToolHints describes optional MCP behavior hints advertised with a tool.
type ToolHints struct {
	// ReadOnly means the recipe does not modify its environment.
	ReadOnly *bool

	// Destructive means the recipe may perform destructive updates.
	Destructive *bool

	// Idempotent means repeated calls with the same args have no additional effect.
	Idempotent *bool

	// OpenWorld means the recipe interacts with external entities.
	OpenWorld *bool
}

// Recipe represents a single parsed Makefile recipe with its full metadata.
type Recipe struct {
	// SourceFile is the path of the Makefile this recipe was read from.
	// Empty when parsed directly from an io.Reader.
	SourceFile string

	// ID is the Makefile target name (e.g. "hello-world").
	ID string

	// Name is the human-friendly tool name sourced from the name annotation.
	Name string

	// Description explains what the recipe does.
	Description string

	// Risk indicates the potential impact of running the recipe.
	Risk RiskLevel

	// Params is the list of declared input parameters.
	//
	// nil means no @param annotation was present at all — a validation error.
	// An empty non-nil slice means @param: none was declared (recipe takes no inputs).
	// A non-empty slice contains the declared parameters.
	Params []Param

	// Output describes what the recipe produces on success.
	Output string

	// OutputType is an HTTP content type hint for successful output,
	// e.g. "text/plain" or "application/json; charset=utf-8".
	// It documents the expected shape of stdout but is not enforced at runtime.
	OutputType string

	// ToolHints are optional MCP behavior hints for clients and inspectors.
	ToolHints ToolHints
}

// Validate returns all field-level violations for the Recipe.
// An empty slice means the Recipe is valid.
// Every missing or invalid field is reported independently so callers receive
// the complete set of problems in a single call.
func (r *Recipe) Validate() []error {
	var errs []error
	if r.ID == "" {
		errs = append(errs, errors.New("recipe is missing an ID (target name)"))
	}
	if r.Name == "" {
		errs = append(errs, errors.New("recipe is missing a name annotation"))
	}
	if r.Description == "" {
		errs = append(errs, errors.New("recipe is missing a description annotation"))
	}
	if r.Params == nil {
		errs = append(errs, errors.New("recipe is missing param declarations; use '@param: none' for recipes with no inputs"))
	}
	if r.Output == "" {
		errs = append(errs, errors.New("recipe is missing an output annotation"))
	}
	if r.OutputType == "" {
		errs = append(errs, errors.New("recipe is missing an output-type annotation"))
	} else if err := validateHTTPContentType(r.OutputType); err != nil {
		errs = append(errs, err)
	}
	switch r.Risk {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		errs = append(errs, errors.New("recipe risk must be one of: low, medium, high"))
	}
	return errs
}

func validateHTTPContentType(value string) error {
	if _, _, err := mime.ParseMediaType(value); err != nil {
		return fmt.Errorf("recipe output-type must be a valid HTTP content type: %w", err)
	}
	return nil
}

// ValidationError holds a single field-level violation for a recipe.
type ValidationError struct {
	RecipeID   string
	SourceFile string
	Err        error
}

func (v ValidationError) Error() string {
	if v.SourceFile != "" {
		return v.SourceFile + ": recipe " + v.RecipeID + ": " + v.Err.Error()
	}
	return "recipe " + v.RecipeID + ": " + v.Err.Error()
}

// ParseResult is returned by ParseMakefile and ParseMakefiles.
type ParseResult struct {
	Recipes []Recipe
	Errors  []ValidationError
}

// Valid reports whether the parse produced zero validation errors.
func (pr *ParseResult) Valid() bool {
	return len(pr.Errors) == 0
}
