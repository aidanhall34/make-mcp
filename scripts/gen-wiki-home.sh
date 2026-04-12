#!/usr/bin/env bash
set -e

WIKI_DIR="wiki"
HOME_FILE="${WIKI_DIR}/Home.md"
README_FILE="README.md"
TEMP_HOME=$(mktemp)

if [ ! -f "$README_FILE" ]; then
    echo "Error: $README_FILE not found."
    exit 1
fi

if [ ! -d "$WIKI_DIR" ]; then
    echo "Error: $WIKI_DIR directory not found."
    exit 1
fi

# Transform README.md into wiki/Home.md format:
#   1. Rewrite image paths: ./wiki/images/ -> ./images/
#   2. Convert wiki page links: [text](https://.../wiki/Page-Name) -> [[Page Name]]
#   3. Convert bare wiki links: [text](https://.../wiki) -> [[Home]]
perl -pe '
    s|\./wiki/images/|./images/|g;
    s|\[([^\]]+)\]\(https://github\.com/aidanhall34/make-mcp/wiki/([^)]+)\)|do { (my $p = $2) =~ s/-/ /g; "[[$p]]" }|ge;
    s|\[([^\]]+)\]\(https://github\.com/aidanhall34/make-mcp/wiki\)|[[Home]]|g;
' "$README_FILE" > "$TEMP_HOME"

if ! cmp -s "$TEMP_HOME" "$HOME_FILE"; then
    mv "$TEMP_HOME" "$HOME_FILE"
    echo "Wiki home page was outdated and has been automatically updated."
    echo "Changes detected in: $HOME_FILE"
    echo "Please stage the changes and commit again."
    exit 1
fi

rm -f "$TEMP_HOME"
