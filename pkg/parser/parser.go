package parser

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ParseOptions configures how a Makefile is parsed.
type ParseOptions struct {
	// Delimiter is the literal string that must appear between the # comment
	// character and an annotation key. Defaults to "@" when empty.
	Delimiter string

	// Strict requires every supported annotation, including optional MCP tool
	// hints, to be explicitly declared on annotated recipes.
	Strict bool
}

// targetRE matches a Makefile target line. Captured group 1 is the target name.
//
// Rules encoded in the pattern:
//   - Line must start with one or more valid target-name characters.
//   - A colon must follow the name.
//   - The character immediately after the colon must not be = (excludes := assignments),
//     or the colon may be at end-of-string.
var targetRE = regexp.MustCompile(`^([-a-zA-Z0-9_./ ]+):(?:[^=]|$)`)

// paramNameRE validates param names.
// Names must start with a letter and contain only letters, digits, hyphens, or underscores.
// Regex is used here because param names are user-defined inputs that require validation.
var paramNameRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// buildAnnotationRE compiles the annotation extraction regex for the given
// delimiter. The delimiter is treated as a literal string (regexp.QuoteMeta).
//
// Pattern:  #.*?<delimiter>\s+([a-zA-Z][a-zA-Z0-9-]*):\s*(.*)
//
//   - #        — comment character
//   - .*?      — non-greedy: anything between # and the first occurrence of the delimiter
//   - \s+      — one or more spaces between delimiter and key (required)
//   - (key)    — captures the annotation key; validated as [a-zA-Z][a-zA-Z0-9-]*
//   - :\s*     — colon plus optional whitespace
//   - (.*)     — captures the value
func buildAnnotationRE(delimiter string) *regexp.Regexp {
	return regexp.MustCompile(`#.*?` + regexp.QuoteMeta(delimiter) + `\s+([a-zA-Z][a-zA-Z0-9-]*):\s*(.*)`)
}

// ParseMakefilePath opens the file at path, parses it, and tags every Recipe
// and ValidationError with its SourceFile path.
func ParseMakefilePath(path string, opts ParseOptions) (*ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	result, err := ParseMakefile(f, opts)
	if err != nil {
		return nil, err
	}
	for i := range result.Recipes {
		result.Recipes[i].SourceFile = path
	}
	for i := range result.Errors {
		result.Errors[i].SourceFile = path
	}
	return result, nil
}

// ParseMakefiles parses all Makefile paths in order and merges their results
// into a single ParseResult.
func ParseMakefiles(paths []string, opts ParseOptions) (*ParseResult, error) {
	merged := &ParseResult{}
	for _, path := range paths {
		r, err := ParseMakefilePath(path, opts)
		if err != nil {
			return nil, err
		}
		merged.Recipes = append(merged.Recipes, r.Recipes...)
		merged.Errors = append(merged.Errors, r.Errors...)
	}
	return merged, nil
}

// ParseMakefile reads a Makefile from r and returns all annotated recipes.
//
// Annotation format (per line):
//
//	#.*?<delimiter>\s+<key>:\s*<value>
//
// The regex is not anchored, so the # may appear anywhere in the line.
// The non-greedy .*? between # and the delimiter means the first occurrence
// of the delimiter after # is used. At least one space must follow the
// delimiter before the key.
//
// The @param key is handled specially: multiple @param lines are collected per
// recipe. All other keys are scalar (last value wins if repeated).
//
// Targets not immediately preceded by a contiguous annotation block are
// silently ignored. Every validation and parse violation for every recipe is
// collected; parsing does not stop at the first error.
func ParseMakefile(r io.Reader, opts ParseOptions) (*ParseResult, error) {
	delim := opts.Delimiter
	if delim == "" {
		delim = "@"
	}

	annotRE := buildAnnotationRE(delim)
	result := &ParseResult{}
	scanner := bufio.NewScanner(r)

	// pendingMeta accumulates scalar key→value pairs from the current annotation block.
	// pendingParamStrings accumulates raw @param values; nil means no @param was seen.
	pendingMeta := map[string]string{}
	var pendingParamStrings []string
	paramsSeen := false
	paramNoneSeen := false
	inAnnotationBlock := false

	resetState := func() {
		pendingMeta = map[string]string{}
		pendingParamStrings = nil
		paramsSeen = false
		paramNoneSeen = false
		inAnnotationBlock = false
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Pre-compute both classifiers so the switch cases stay flat.
		annotKey, annotVal, isAnnotation := extractAnnotation(line, annotRE)
		targetName, isTarget := matchTargetLine(trimmed)

		switch {
		case isAnnotation:
			// @param is a repeatable key; all others are scalar (map, last wins).
			if annotKey == "param" {
				paramsSeen = true
				if annotVal == "none" {
					paramNoneSeen = true
				} else {
					pendingParamStrings = append(pendingParamStrings, annotVal)
				}
			} else {
				pendingMeta[annotKey] = annotVal
			}
			inAnnotationBlock = true

		case strings.HasPrefix(trimmed, "#"):
			// Plain comment with no valid annotation — does not reset the block.

		case isTarget:
			if !inAnnotationBlock || len(pendingMeta) == 0 && !paramsSeen {
				resetState()
				continue
			}

			// .PHONY and other dot-prefixed special targets are not recipes.
			if strings.HasPrefix(targetName, ".") {
				resetState()
				continue
			}

			params, paramErrs := buildParams(targetName, paramsSeen, paramNoneSeen, pendingParamStrings)
			hints, hintErrs := buildToolHints(targetName, pendingMeta)
			strictErrs := validateStrictAnnotations(targetName, pendingMeta, opts.Strict)
			recipe := Recipe{
				ID:          targetName,
				Name:        pendingMeta["name"],
				Description: pendingMeta["description"],
				Risk:        RiskLevel(pendingMeta["risk"]),
				Params:      params,
				Output:      pendingMeta["output"],
				OutputType:  pendingMeta["output-type"],
				ToolHints:   hints,
			}
			result.Recipes = append(result.Recipes, recipe)
			result.Errors = append(result.Errors, paramErrs...)
			result.Errors = append(result.Errors, hintErrs...)
			result.Errors = append(result.Errors, strictErrs...)

			// Collect every schema violation; do not stop at the first one.
			for _, err := range recipe.Validate() {
				result.Errors = append(result.Errors, ValidationError{
					RecipeID: targetName,
					Err:      err,
				})
			}

			resetState()

		case trimmed == "":
			// A blank line between an annotation block and its target resets state.
			if inAnnotationBlock {
				resetState()
			}

		default:
			// Variable assignments, includes, and other directives reset state.
			resetState()
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning makefile: %w", err)
	}

	return result, nil
}

func validateStrictAnnotations(targetID string, meta map[string]string, strict bool) []ValidationError {
	if !strict {
		return nil
	}

	required := []string{"read-only", "destructive", "idempotent", "open-world"}
	var errs []ValidationError
	for _, key := range required {
		if _, ok := meta[key]; !ok {
			errs = append(errs, ValidationError{
				RecipeID: targetID,
				Err:      fmt.Errorf("recipe is missing a %s annotation required by strict mode", key),
			})
		}
	}
	return errs
}

func buildToolHints(targetID string, meta map[string]string) (ToolHints, []ValidationError) {
	var hints ToolHints
	var errs []ValidationError

	readOnly, err := optionalBoolAnnotation(meta, "read-only")
	errs = appendBoolAnnotationError(errs, targetID, err)
	destructive, err := optionalBoolAnnotation(meta, "destructive")
	errs = appendBoolAnnotationError(errs, targetID, err)
	idempotent, err := optionalBoolAnnotation(meta, "idempotent")
	errs = appendBoolAnnotationError(errs, targetID, err)
	openWorld, err := optionalBoolAnnotation(meta, "open-world")
	errs = appendBoolAnnotationError(errs, targetID, err)

	hints.ReadOnly = readOnly
	hints.Destructive = destructive
	hints.Idempotent = idempotent
	hints.OpenWorld = openWorld

	return hints, errs
}

func optionalBoolAnnotation(meta map[string]string, key string) (*bool, error) {
	raw, ok := meta[key]
	if !ok {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fmt.Errorf("@%s %q: expected boolean value 'true' or 'false'", key, raw)
	}
	return &value, nil
}

func appendBoolAnnotationError(errs []ValidationError, targetID string, err error) []ValidationError {
	if err == nil {
		return errs
	}
	return append(errs, ValidationError{RecipeID: targetID, Err: err})
}

// buildParams converts the raw accumulated @param state into a Params slice
// and any parse-level ValidationErrors. It is called once per recipe.
//
//   - seen=false  → nil Params (Validate will report "missing param declarations")
//   - noneeSeen=true, no raw vals → []Param{} (explicitly no inputs)
//   - noneSeen=true with raw vals → error (cannot mix @param: none with named params)
//   - raw vals only → parsed into []Param, parse errors collected separately
func buildParams(targetID string, seen, noneSeen bool, rawVals []string) ([]Param, []ValidationError) {
	if !seen {
		return nil, nil
	}
	if noneSeen && len(rawVals) > 0 {
		return []Param{}, []ValidationError{{
			RecipeID: targetID,
			Err:      fmt.Errorf("cannot mix '@param: none' with named @param declarations"),
		}}
	}
	if noneSeen {
		return []Param{}, nil
	}

	var params []Param
	var errs []ValidationError
	for _, raw := range rawVals {
		p, err := parseParam(raw)
		if err != nil {
			errs = append(errs, ValidationError{RecipeID: targetID, Err: err})
		} else {
			params = append(params, p)
		}
	}
	// Ensure non-nil so Validate() does not also report "missing param declarations".
	if params == nil {
		params = []Param{}
	}
	return params, errs
}

// parseParam parses a single @param annotation value of the form:
//
//	<name> <type> | <description>
//
// The name is validated against paramNameRE. The type must be one of: string, int, bool.
// The pipe separator must be surrounded by spaces.
func parseParam(val string) (Param, error) {
	parts := strings.SplitN(val, " | ", 2)
	if len(parts) != 2 {
		return Param{}, fmt.Errorf("@param %q: expected format '<name> <type> | <description>'", val)
	}
	nameType := strings.Fields(parts[0])
	if len(nameType) != 2 {
		return Param{}, fmt.Errorf("@param %q: expected '<name> <type>' before '|'", val)
	}
	name, pType := nameType[0], ParamType(nameType[1])
	desc := strings.TrimSpace(parts[1])

	// Param names are user-defined identifiers — validate with a regex.
	if !paramNameRE.MatchString(name) {
		return Param{}, fmt.Errorf("@param name %q: must match [a-zA-Z][a-zA-Z0-9_-]*", name)
	}
	switch pType {
	case ParamTypeString, ParamTypeInt, ParamTypeBool:
	default:
		return Param{}, fmt.Errorf("@param %q: type %q must be one of: string, int, bool", name, pType)
	}
	return Param{Name: name, Type: pType, Description: desc}, nil
}

// extractAnnotation extracts the annotation key and value from a single line
// using a pre-compiled regex. Returns ok=false if the line does not contain a
// valid annotation. The full parse — delimiter location, space enforcement, key
// validation, and value capture — is performed in a single FindStringSubmatch call.
func extractAnnotation(line string, re *regexp.Regexp) (key, value string, ok bool) {
	m := re.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}

// matchTargetLine reports whether trimmed looks like a Makefile target definition
// and, if so, returns the target name. Detection and extraction are done in a
// single FindStringSubmatch call on the package-level targetRE.
func matchTargetLine(trimmed string) (name string, ok bool) {
	m := targetRE.FindStringSubmatch(trimmed)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}
