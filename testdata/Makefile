SHELL=/usr/bin/env sh

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
