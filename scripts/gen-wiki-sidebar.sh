#!/usr/bin/env bash
set -e

# Configuration
WIKI_DIR="wiki"
SIDEBAR_FILE="${WIKI_DIR}/_Sidebar.md"
TEMP_SIDEBAR=$(mktemp)

# Ensure wiki directory exists
if [ ! -d "$WIKI_DIR" ]; then
    echo "Error: $WIKI_DIR directory not found."
    exit 1
fi

# Start with Home
echo "- [[Home]]" > "$TEMP_SIDEBAR"

# Add other pages except Home and Sidebar itself, sorted alphabetically
# Using find with -print0 and sort -z to safely handle spaces in filenames
find "$WIKI_DIR" -maxdepth 1 -name "*.md" ! -name "Home.md" ! -name "_Sidebar.md" -print0 | \
    sort -z | \
    while IFS= read -r -d '' file; do
        page=$(basename "$file" .md)
        echo "- [[$page]]" >> "$TEMP_SIDEBAR"
    done

# Check if the generated sidebar differs from the existing one
if ! cmp -s "$TEMP_SIDEBAR" "$SIDEBAR_FILE"; then
    mv "$TEMP_SIDEBAR" "$SIDEBAR_FILE"
    echo "Wiki sidebar was outdated and has been automatically updated."
    echo "Changes detected in: $SIDEBAR_FILE"
    echo "Please stage the changes and commit again."
    exit 1
fi

rm -f "$TEMP_SIDEBAR"
