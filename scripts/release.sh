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

TODAY=$(date '+%Y-%m-%d')
VERSION_DOC="${ROOT_DIR}/docs/changelog/v${NEW_VERSION}.md"

# 1. Update version across all project components & changelogs in a single unified step
echo -e "${BLUE}->${NC} Updating version to v${NEW_VERSION} across codebase and changelogs..."
python3 -c '
import sys, os, re, json

ver = sys.argv[1]
root = sys.argv[2]
today = sys.argv[3]

# 1. Update Go version in version.go
go_ver_path = f"{root}/version.go"
if os.path.isfile(go_ver_path):
    with open(go_ver_path, "r", encoding="utf-8") as f:
        text = f.read()
    text = re.sub(r"const Version = \"[^\"]+\"", f"const Version = \"{ver}\"", text)
    with open(go_ver_path, "w", encoding="utf-8") as f:
        f.write(text)

# 2. Update MCP VERSION in mcp/server.py
mcp_path = f"{root}/mcp/server.py"
if os.path.isfile(mcp_path):
    with open(mcp_path, "r", encoding="utf-8") as f:
        text = f.read()
    text = re.sub(r"VERSION\s*=\s*\"[^\"]+\"", f"VERSION = \"{ver}\"", text)
    with open(mcp_path, "w", encoding="utf-8") as f:
        f.write(text)

# 3. Update version in ide-extension/package.json
pkg_path = f"{root}/ide-extension/package.json"
if os.path.isfile(pkg_path):
    with open(pkg_path, "r", encoding="utf-8") as f:
        pkg = json.load(f)
    pkg["version"] = ver
    with open(pkg_path, "w", encoding="utf-8") as f:
        json.dump(pkg, f, indent=2)
        f.write("\n")

# 4. Create version-specific doc under docs/changelog/vX.Y.Z.md if not exists
doc_dir = f"{root}/docs/changelog"
os.makedirs(doc_dir, exist_ok=True)
doc_path = f"{doc_dir}/v{ver}.md"
if not os.path.isfile(doc_path):
    with open(doc_path, "w", encoding="utf-8") as f:
        f.write(f"# Release v{ver} ({today})\n\n## Changes in this release\n- Antigravity Dump Guard & SSD TBW Watchdog release v{ver}.\n")

# 5. Update main CHANGELOG.md index
cl_path = f"{root}/CHANGELOG.md"
if os.path.isfile(cl_path):
    with open(cl_path, "r", encoding="utf-8") as f:
        content = f.read()
    entry = f"- **[v{ver}](docs/changelog/v{ver}.md)** ({today}) - Release v{ver}."
    if f"v{ver}" not in content:
        lines = content.splitlines(keepends=True)
        new_lines = []
        for line in lines:
            new_lines.append(line)
            if line.strip() == "## Releases":
                new_lines.append(f"{entry}\n")
        with open(cl_path, "w", encoding="utf-8") as f:
            f.writelines(new_lines)
' "${NEW_VERSION}" "${ROOT_DIR}" "${TODAY}"

# 2. Run checks, tests, and security audit
echo -e "${BLUE}->${NC} Running verification tests & pre-release security audit..."
(cd "$ROOT_DIR" && make check && make test && make audit)

# 5. Git commit and tag
echo -e "${BLUE}->${NC} Staging files..."
git add -A "${ROOT_DIR}"

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
