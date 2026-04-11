# Output Types

`@ output-type` documents the expected content type for a recipe's successful
stdout output.

Example:

```make
# @ output: A JSON document describing the deployment
# @ output-type: application/json
deploy-status:
	@echo '{"status":"ok"}'
```

## Rules

- `@ output-type` is required for every annotated recipe.
- The value must be a valid HTTP content type.
- Validation uses standard media type parsing, so parameters such as
  `charset=utf-8` are allowed.

Valid examples:

- `text/plain`
- `application/json`
- `application/json; charset=utf-8`
- `application/octet-stream`

Invalid examples:

- `json`
- `plain/text`
- `not a content type`

## Behavior

`@ output-type` is a type hint for tools and clients. It is used to document the
expected output format in tool metadata.

It is not used to validate the actual process output at runtime. A recipe that
declares `application/json` is still allowed to print invalid JSON; the server
will return the command output without enforcing conformance to the declared
content type.
