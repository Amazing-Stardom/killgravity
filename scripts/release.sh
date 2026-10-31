#!/usr/bin/env bash
#
# scripts/release.sh - Release automation script for killgravity
#
# Copyright (c) 2026 Ganesh Kumar
# Licensed under the MIT License.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="${SCRIPT_DIR}/.."

# Formatting
if [ -t 1 ]; then
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    RED='\033[0;31m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    GREEN=''
    YELLOW=''
    RED=''
    BLUE=''
    BOLD=''
    NC=''
fi

if [ $# -lt 1 ]; then
    echo -e "${RED}Error: Version number required.${NC}"
    echo "Usage: ./scripts/release.sh <X.Y.Z>"
    echo "Example: ./scripts/release.sh 1.1.0"
    exit 1
fi

NEW_VERSION="${1#v}" # Strip leading 'v' if provided

# Validate SemVer format
if ! [[ "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then
    echo -e "${RED}Error: Invalid semantic version '${NEW_VERSION}'. Expected format: X.Y.Z (e.g. 1.1.0)${NC}"
    exit 1
fi

echo -e "${BOLD}Preparing release for v${NEW_VERSION}...${NC}"

# Check for uncommitted changes
if ! git diff-index --quiet HEAD -- 2>/dev/null; then
    echo -e "${YELLOW}Warning: Working directory has uncommitted changes.${NC}"
    git status --short
    read -rp "Do you want to proceed and include these changes? (y/N) " confirm
    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 1
    fi
fi

# 1. Update version in bin/killgravity and mcp/server.py
echo -e "${BLUE}->${NC} Updating version in bin/killgravity and mcp/server.py..."
python3 -c '
import sys, re
for path in sys.argv[2:]:
    with open(path, "r", encoding="utf-8") as f:
        text = f.read()
    text = re.sub(r"VERSION\s*=\s*\"[^\"]+\"", f"VERSION = \"{sys.argv[1]}\"", text)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
' "${NEW_VERSION}" "${ROOT_DIR}/bin/killgravity" "${ROOT_DIR}/mcp/server.py"

# 2. Run checks, tests, and security audit
echo -e "${BLUE}->${NC} Running verification tests & pre-release security audit..."
(cd "$ROOT_DIR" && make check && make test && make audit)

# 3. Create version-specific doc under docs/changelog/
CHANGELOG_DIR="${ROOT_DIR}/docs/changelog"
mkdir -p "$CHANGELOG_DIR"
VERSION_DOC="${CHANGELOG_DIR}/v${NEW_VERSION}.md"
TODAY=$(date '+%Y-%m-%d')

if [ ! -f "$VERSION_DOC" ]; then
    echo -e "${BLUE}->${NC} Generating ${VERSION_DOC}..."
    cat <<EOF > "$VERSION_DOC"
# Release v${NEW_VERSION} (${TODAY})

## Changes in this release
- Maintenance release and updates.
EOF
fi

# 4. Update main CHANGELOG.md index
if ! grep -q "v${NEW_VERSION}" "${ROOT_DIR}/CHANGELOG.md"; then
    echo -e "${BLUE}->${NC} Adding v${NEW_VERSION} to CHANGELOG.md..."
    python3 -c '
import sys
path = sys.argv[1]
entry = sys.argv[2]
with open(path, "r", encoding="utf-8") as f:
    lines = f.readlines()
new_lines = []
for line in lines:
    new_lines.append(line)
    if line.strip() == "## Releases":
        new_lines.append(f"- {entry}\n")
with open(path, "w", encoding="utf-8") as f:
    f.writelines(new_lines)
' "${ROOT_DIR}/CHANGELOG.md" "**[v${NEW_VERSION}](docs/changelog/v${NEW_VERSION}.md)** (${TODAY}) - Release v${NEW_VERSION}."
fi

# 5. Git commit and tag
echo -e "${BLUE}->${NC} Staging files..."
git add "${ROOT_DIR}/bin/killgravity" "${ROOT_DIR}/CHANGELOG.md" "${VERSION_DOC}"

if git diff --staged --quiet; then
    echo -e "${YELLOW}No changes to commit (already up-to-date).${NC}"
else
    git commit -m "chore(release): bump version to v${NEW_VERSION}"
fi

TAG_NAME="v${NEW_VERSION}"
if git rev-parse "$TAG_NAME" >/dev/null 2>&1; then
    echo -e "${YELLOW}Tag ${TAG_NAME} already exists. Skipping tag creation.${NC}"
else
    echo -e "${BLUE}->${NC} Creating annotated git tag ${TAG_NAME}..."
    git tag -a "$TAG_NAME" -m "Release ${TAG_NAME}"
fi

echo -e "\n${GREEN}${BOLD}✓ Release ${TAG_NAME} prepared successfully!${NC}"
echo -e "To publish this release to GitHub, run:"
echo -e "  git push origin main"
echo -e "  git push origin ${TAG_NAME}"
