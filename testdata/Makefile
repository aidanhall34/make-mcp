SHELL=/usr/bin/env sh

.PHONY: all
all: hello-world

.PHONY: test
test: hello-world

# @ name: Hello World
# @ description: Prints a friendly greeting to stdout. Useful for verifying the MCP server is running correctly.
# @ risk: low
# @ param: none
# @ output: A greeting message string
# @ output-type: text/plain
hello-world:
	@echo "Hello, World!"

# @ name: Clean Build Artifacts
# @ description: Removes all compiled binaries and build cache from the project directory.
# @ risk: medium
# @ param: none
# @ output: Confirmation that artifacts were removed
# @ output-type: text/plain
.PHONY: clean
clean:
	@rm -rf ./bin ./dist
	@echo "Build artifacts removed."

# @ name: Greet User
# @ description: Sends a personalised greeting to stdout. Demonstrates named input parameters.
# @ risk: low
# @ param: name string | The name to include in the greeting
# @ param: count int | Number of times to repeat the greeting (default: 1)
# @ output: One greeting line per repetition
# @ output-type: text/plain
greet:
	@for i in $(shell seq 1 $(COUNT)); do echo "Hello, $(NAME)!"; done

# @ name: Always Fail
# @ description: Calls a command that does not exist, causing make to exit with a non-zero status. Used for error-handling compliance tests.
# @ risk: low
# @ param: none
# @ output: Nothing — the recipe always fails before producing output
# @ output-type: text/plain
always-fail:
	@make-mcp-no-such-command-xyz

.PHONY: slow-stream
# @ name: Slow Stream
# @ description: Prints output in three chunks with short delays to demonstrate progress notification streaming.
# @ risk: low
# @ param: none
# @ output: Three progress chunks followed by a completion message
# @ output-type: text/plain
slow-stream:
	@for i in 1 2 3; do printf "chunk $$i\n"; sleep 0.1; done
	@printf "done\n"

.PHONY: slow-stream-fail
# @ name: Slow Stream Fail
# @ description: Prints output in three chunks with short delays then exits non-zero. Used to validate that failure notifications are handled correctly during streaming.
# @ risk: low
# @ param: none
# @ output: Three progress chunks followed by a non-zero exit
# @ output-type: text/plain
slow-stream-fail:
	@for i in 1 2 3; do printf "chunk $$i\n"; sleep 0.1; done
	@exit 1
