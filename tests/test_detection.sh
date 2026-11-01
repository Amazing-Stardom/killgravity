#!/usr/bin/env bash
#
# tests/test_detection.sh - Smoke test and detection logic verification
#
# Copyright (c) 2026 Ganesh Kumar
# Licensed under the MIT License.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KILLGRAVITY="${SCRIPT_DIR}/../bin/killgravity"

PASSED=0
FAILED=0

assert_success() {
    local desc="$1"
    shift
    echo -n "Testing: $desc ... "
    if "$@" >/dev/null 2>&1; then
        echo "PASS"
        PASSED=$((PASSED + 1))
    else
        echo "FAIL"
        FAILED=$((FAILED + 1))
    fi
}

echo "=== killgravity Test Suite ==="

assert_success "Version command" "$KILLGRAVITY" version
assert_success "Help command" "$KILLGRAVITY" help
assert_success "Config command" "$KILLGRAVITY" config
assert_success "Status command" "$KILLGRAVITY" status
assert_success "Test probe command" "$KILLGRAVITY" test

# Test dummy process detection and termination
echo -n "Testing: Dummy process termination ... "
# Spawn a dummy sleep process with unique name in cmdline
bash -c 'exec -a "mock-antigravity-ide-test" sleep 30' &
MOCK_PID=$!

# Wait for process to start and exec to mock name
STARTED=false
for _ in {1..30}; do
    if grep -q "mock-antigravity-ide-test" "/proc/$MOCK_PID/cmdline" 2>/dev/null && [ "$(basename "$(readlink -f "/proc/$MOCK_PID/exe" 2>/dev/null || true)")" != "bash" ]; then
        STARTED=true
        break
    fi
    sleep 0.1
done

if [ "$STARTED" = "true" ]; then
    # Use killgravity kill with custom pattern
    "$KILLGRAVITY" --pattern "mock-antigravity-ide-test" kill >/dev/null 2>&1

    # Wait for process to terminate
    TERMINATED=false
    for _ in {1..30}; do
        if ! kill -0 "$MOCK_PID" 2>/dev/null; then
            TERMINATED=true
            break
        fi
        sleep 0.1
    done

    if [ "$TERMINATED" = "true" ]; then
        echo "PASS (Process killed successfully)"
        PASSED=$((PASSED + 1))
    else
        echo "FAIL (Process still alive)"
        kill -9 "$MOCK_PID" 2>/dev/null || true
        FAILED=$((FAILED + 1))
    fi
else
    echo "SKIP (Could not launch mock process)"
fi

echo "==============================="
echo "Results: $PASSED passed, $FAILED failed."

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi

