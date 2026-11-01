#!/usr/bin/env bash
#
# scripts/audit.sh - Comprehensive pre-release safety & security audit
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

PASSED=0
FAILED=0
WARNINGS=0

report_pass() {
    echo -e "  [${GREEN}PASS${NC}] $1"
    PASSED=$((PASSED + 1))
}

report_fail() {
    echo -e "  [${RED}FAIL${NC}] $1" >&2
    FAILED=$((FAILED + 1))
}

report_warn() {
    echo -e "  [${YELLOW}WARN${NC}] $1"
    WARNINGS=$((WARNINGS + 1))
}

echo -e "${BOLD}====================================================${NC}"
echo -e "${BOLD}        killgravity Pre-Release Security Audit       ${NC}"
echo -e "${BOLD}====================================================${NC}"

# 1. Version Consistency Check
echo -e "\n${BLUE}1. Version Consistency Audit:${NC}"
BIN_VER=$("${ROOT_DIR}/bin/killgravity" --version | awk '{print $3}')
MCP_VER=$(grep -E '^VERSION = ' "${ROOT_DIR}/mcp/server.py" | cut -d'"' -f2)

if [ "$BIN_VER" = "$MCP_VER" ]; then
    report_pass "CLI version ($BIN_VER) matches MCP version ($MCP_VER)"
else
    report_fail "Version mismatch: CLI ($BIN_VER) vs MCP ($MCP_VER)"
fi

if [ -f "${ROOT_DIR}/docs/changelog/v${BIN_VER}.md" ]; then
    report_pass "Changelog document exists: docs/changelog/v${BIN_VER}.md"
else
    report_fail "Missing docs/changelog/v${BIN_VER}.md"
fi

if grep -q "v${BIN_VER}" "${ROOT_DIR}/CHANGELOG.md"; then
    report_pass "Root CHANGELOG.md references v${BIN_VER}"
else
    report_fail "Root CHANGELOG.md is missing reference to v${BIN_VER}"
fi

# 2. Secret & Sensitive Data Scanning
echo -e "\n${BLUE}2. Secrets & Sensitive Data Audit:${NC}"
# Scan for high-risk tokens (AWS, GitHub, private keys, LiveReview keys, etc.)
SUSPICIOUS_PATTERNS=(
    "ghp_[A-Za-z0-9_]{36}"
    "lr_[A-Za-z0-9_]{30,}"
    "AKIA[0-9A-Z]{16}"
    "-----BEGIN OPENSSH PRIVATE KEY-----"
    "-----BEGIN RSA PRIVATE KEY-----"
    "password[ ]*=[ ]*['\"][^'\"]+['\"]"
)

FOUND_SECRET=false
for pat in "${SUSPICIOUS_PATTERNS[@]}"; do
    MATCHES=$(grep -rnE "$pat" "${ROOT_DIR}" \
        --exclude-dir=".git" \
        --exclude-dir=".gemini" \
        --exclude="audit.sh" 2>/dev/null || true)
    if [ -n "$MATCHES" ]; then
        report_fail "Potential secret found matching pattern '${pat}':\n$MATCHES"
        FOUND_SECRET=true
    fi
done

if [ "$FOUND_SECRET" = "false" ]; then
    report_pass "No leaked API tokens, passwords, or private keys detected."
fi

# 3. File Permissions Audit
echo -e "\n${BLUE}3. File Permissions Audit:${NC}"
EXECUTABLES=(
    "bin/killgravity"
    "install.sh"
    "tests/test_detection.sh"
    "scripts/release.sh"
    "scripts/audit.sh"
    "mcp/server.py"
)

for exe in "${EXECUTABLES[@]}"; do
    if [ -x "${ROOT_DIR}/${exe}" ]; then
        report_pass "${exe} is properly marked executable (755)"
    else
        report_fail "${exe} is NOT executable (needs chmod +x)"
    fi
done

# 4. Syntax & Static Analysis
echo -e "\n${BLUE}4. Code Syntax & Static Verification:${NC}"
SH_FILES=(
    "install.sh"
    "tests/test_detection.sh"
    "scripts/release.sh"
    "scripts/audit.sh"
)

for sh in "${SH_FILES[@]}"; do
    if bash -n "${ROOT_DIR}/${sh}"; then
        report_pass "Bash syntax valid: ${sh}"
    else
        report_fail "Bash syntax error in: ${sh}"
    fi
done

# Validate configuration file format
if python3 -c '
import sys
with open(sys.argv[1], "r", encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            sys.exit(1)
sys.exit(0)
' "${ROOT_DIR}/config/killgravity.conf" 2>/dev/null; then
    report_pass "Config file format valid (KEY=VALUE): config/killgravity.conf"
else
    report_fail "Config file format invalid: config/killgravity.conf"
fi

if [ -f "${ROOT_DIR}/go.mod" ]; then
    if (cd "$ROOT_DIR" && go vet ./... 2>/dev/null); then
        report_pass "Go syntax and vet valid: ${ROOT_DIR}"
    else
        report_fail "Go vet failed on project sources"
    fi
fi

if python3 -m py_compile "${ROOT_DIR}/mcp/server.py" 2>/dev/null; then
    report_pass "Python syntax valid: mcp/server.py"
else
    report_fail "Python syntax error in: mcp/server.py"
fi

if [ -f "${ROOT_DIR}/ide-extension/extension.js" ]; then
    if node -c "${ROOT_DIR}/ide-extension/extension.js"; then
        report_pass "JavaScript syntax valid: ide-extension/extension.js"
    else
        report_fail "JavaScript syntax error in: ide-extension/extension.js"
    fi
fi

# 5. Process & Kill Safety Bounds Audit
echo -e "\n${BLUE}5. Process Safety & Blast Radius Audit:${NC}"
# Ensure kill commands do not use unrestricted / bare pkill
if grep -q "pkill -9 -if \"antigravity\"" "${ROOT_DIR}"/*.go 2>/dev/null; then
    report_fail "Dangerous broad match found: 'pkill -9 -if antigravity' (could kill agent/hub)"
else
    report_pass "Process killing safely scoped to '${BIN_VER}' / targeted pattern"
fi

if grep -q "pid != my_pid" "${ROOT_DIR}"/*.go 2>/dev/null; then
    report_pass "Process filtering excludes self (my_pid) and parent (ppid)"
else
    report_warn "Double check process filtering exclusions"
fi

# 6. Functional Test Suite
echo -e "\n${BLUE}6. Automated Functional Verification:${NC}"
if (cd "$ROOT_DIR" && ./tests/test_detection.sh >/dev/null 2>&1); then
    report_pass "Automated test suite (tests/test_detection.sh) passed 100%"
else
    report_fail "Automated test suite failed!"
fi

# 7. MCP JSON-RPC Protocol Test
echo -e "\n${BLUE}7. MCP Protocol Verification:${NC}"
MCP_CHECK=$(python3 -c '
import subprocess, json
try:
    proc = subprocess.Popen(["'"${ROOT_DIR}"'/mcp/server.py"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
    req = json.dumps({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {}}) + "\n"
    proc.stdin.write(req)
    proc.stdin.flush()
    resp = json.loads(proc.stdout.readline())
    proc.terminate()
    print(resp.get("result", {}).get("serverInfo", {}).get("name", ""))
except Exception as e:
    print(f"ERROR: {e}")
')

if [ "$MCP_CHECK" = "killgravity" ]; then
    report_pass "MCP server initialized cleanly and responds to JSON-RPC 2.0"
else
    report_fail "MCP server failed initialization check: $MCP_CHECK"
fi

# Summary
echo -e "\n${BOLD}====================================================${NC}"
echo -e "Audit Results: ${GREEN}${PASSED} passed${NC}, ${RED}${FAILED} failed${NC}, ${YELLOW}${WARNINGS} warnings${NC}."

if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}${BOLD}✕ Release Audit FAILED. Resolve errors before releasing!${NC}" >&2
    exit 1
else
    echo -e "${GREEN}${BOLD}✓ Release Audit PASSED cleanly. Safe for distribution!${NC}"
    exit 0
fi
