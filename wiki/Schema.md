# Schema

Metadata for each recipe is provided as structured comments placed **directly
above** the recipe target line with no blank lines between the block and the
target.

The declaration must be attached to the real recipe target, not to helper lines
such as `.PHONY`. Unannotated targets are ignored, so this is an intentional
tradeoff: make-mcp only discovers recipes you explicitly annotate.

## Comment format

```
.*#<delimiter> <key>: <value>
```

| Part | Description |
|---|---|
| `.*` | Optional prefix — allows annotations on the same line as other content (inline comments). |
| `#` | The Makefile comment character. |
| `<delimiter>` | A **literal string** configured via `make-mcp.yml` or `--delimiter`. Default: `@`. |
| ` ` | **One or more spaces** must separate the delimiter from the key. |
| `<key>` | A string matching `[a-zA-Z][a-zA-Z0-9\-]*`. |
| `<value>` | The rest of the line after the first `:`. |

## Required annotation fields

| Key | Repeatable | Description |
|---|---|---|
| `name` | No | Human-friendly tool name shown to the MCP client. |
| `description` | No | What the recipe does and when to use it. |
| `risk` | No | Impact level if the recipe is run: `low`, `medium`, or `high`. |
| `param` | Yes | One entry per input, or `@param: none` if the recipe takes no inputs. At least one `@param` line is required. |
| `output` | No | What the recipe produces on success. |
| `output-type` | No | MIME type of the output, e.g. `text/plain`, `application/json`. |

Recipes that are missing any required field, have an invalid `risk` value, or
have a malformed `@param` line are reported as validation errors.

Unannotated targets are silently ignored — they are not exposed as MCP tools.

## Param value format

Inputs are declared with one `@param` line per argument. The value field has
the fixed format `<name> <type> | <description>`:

```
<name> <type> | <description>
```

| Part | Description |
|---|---|
| `<name>` | Identifier for the input; matches `[a-zA-Z][a-zA-Z0-9_-]*`. Maps to the environment variable or Make variable the recipe reads. |
| `<type>` | One of `string`, `int`, `bool`. |
| `\|` | Literal pipe character separating type from description. Must be surrounded by spaces. |
| `<description>` | Free text describing the input, including any default value or constraints. |

Use `@param: none` to declare that the recipe takes no inputs. This is required;
omitting all `@param` lines is a validation error. `@param: none` and named
`@param` lines cannot be mixed in the same recipe.

## Output type

`@ output-type` documents the expected content type for a recipe's successful
stdout output.

- `@ output-type` is required for every annotated recipe.
- The value must be a valid HTTP content type.
- Validation uses standard media type parsing, so parameters such as
  `charset=utf-8` are allowed.
- It is a type hint for tools and clients only — the server does not validate
  actual process output at runtime against the declared type.

Valid examples:

- `text/plain`
- `application/json`
- `application/json; charset=utf-8`
- `application/octet-stream`

Invalid examples:

- `json`
- `plain/text`
- `not a content type`

## Optional MCP tool hints

The MCP `annotations` object can advertise behavior hints to clients. These
values are hints for clients and inspectors; make-mcp does not use them for
access control or sandboxing.

Set `strict: true` or pass `--strict` to require all four hint annotations on
every annotated recipe.

| Key | Default | Description |
|---|---:|---|
| `read-only` | `false` | `true` means the recipe is expected not to modify files, services, external state, or other environment state. |
| `destructive` | `true` for `risk: high`; otherwise `false` | `true` means the recipe may perform destructive updates such as deletion, replacement, shutdown, or rollback. |
| `idempotent` | `false` | `true` means repeated calls with the same arguments are expected to have no additional effect. |
| `open-world` | `true` | `true` means the recipe may interact with external systems such as networks, containers, cloud APIs, package registries, or databases. |

## Examples

### No inputs

```makefile
.PHONY: hello-world

# @ name: Hello World
# @ description: Prints a friendly greeting to stdout.
# @ risk: low
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
hello-world:
	@echo "Hello, World!"
```

The annotation block must be placed between `.PHONY` and the target. If it
appears before `.PHONY`, the annotation is consumed by `.PHONY` and the recipe
is not discovered as an MCP tool:

```makefile
# Incorrect — annotation is consumed by .PHONY, recipe is not discovered
# @ name: Hello World
# @ description: Prints a friendly greeting to stdout.
# @ risk: low
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
.PHONY: hello-world
hello-world:
	@echo "Hello, World!"
```

### With inputs

Multiple `@param` lines are allowed:

```makefile
# @ name: Greet User
# @ description: Sends a personalised greeting to stdout.
# @ risk: low
# @ param: name string | The name to include in the greeting
# @ param: count int | Number of times to repeat the greeting (default: 1)
# @ output: One greeting line per repetition
# @ output-type: text/plain
greet:
	@for i in $(shell seq 1 $(COUNT)); do echo "Hello, $(NAME)!"; done
```

### With MCP tool hints

```makefile
# @ name: Test
# @ description: Runs the unit-test suite.
# @ risk: low
# @ read-only: true
# @ destructive: false
# @ idempotent: true
# @ open-world: false
# @ param: none
# @ output: Test results
# @ output-type: text/plain
tests:
	go test -race ./...
```

### With output type

```makefile
# @ output: A JSON document describing the deployment
# @ output-type: application/json
deploy-status:
	@echo '{"status":"ok"}'
```
