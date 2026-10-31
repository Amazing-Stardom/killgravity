# LiveReview (`git-lrc`) Audit & Remediation Log

This document records the issues identified and remediated across consecutive review cycles using **`git-lrc`** ([LiveReview](https://livereview.hexmos.com)) prior to the v1.0.0 release of **killgravity**.

Using `lrc review --agent-mode`, automated AI-assisted static analysis and diff review helped uncover **8 critical and quality-of-life issues** across testing, installation safety, process isolation, and API compliance.

---

## Issue Summary Matrix

| # | Component | Severity | Category | Issue Identified by `git-lrc` | Resolution Status |
|---|-----------|----------|----------|--------------------------------|-------------------|
| **1** | `install.sh` | **High** | Security / Isolation | Root configuration pollution during `--system` install | **Fixed** |
| **2** | `tests/test_detection.sh` | **Medium** | Reliability / Concurrency | Fixed `sleep` race conditions in process detection | **Fixed** |
| **3** | `bin/killgravity` | **High** | Process Safety | Loose `pkill` fallback terminating caller processes | **Fixed** |
| **4** | `.github/workflows/ci.yml`| **Medium** | CI / Reliability | Missing `set -e` in multi-line CI step | **Fixed** |
| **5** | `mcp/server.py` | **Medium** | Spec Compliance | Silent swallowing of malformed JSON-RPC requests | **Fixed** |
| **6** | `tests/test_detection.sh` | **Medium** | Error Handling | `|| true` suppressing test failures | **Fixed** |
| **7** | `install.sh` | **Low** | Observability | Blanket error suppression (`2>/dev/null`) during service reload | **Fixed** |
| **8** | `bin/killgravity` | **High** | Blast Radius | Broad pattern matching (`antigravity` vs `antigravity-ide`) | **Fixed** |
| **9** | `bin/killgravity` | **High** | Type Safety | Missing type conversion/validation in `set_value` | **Fixed** |
| **10** | `scripts/audit.sh` | **Medium** | Correctness | Inappropriate `bash -n` check on `config/killgravity.conf` | **Fixed** |
| **11** | `scripts/release.sh` | **High** | Portability | Non-portable `sed -i` usage for version bumping | **Fixed** |
| **12** | `scripts/release.sh` | **High** | Portability | Non-portable `sed -i` usage for changelog insertion | **Fixed** |

---

## Detailed Review Breakdown

### 1. Root Configuration Pollution during System Install
- **Review**: [ID 13568](https://livereview.hexmos.com/#/reviews/13568)
- **File**: [`install.sh`](../../install.sh)
- **Flagged by `git-lrc`**: `register_mcp_config` was invoked unconditionally, writing MCP configurations into `/root/.gemini/config/mcp_config.json` when executed under `sudo ./install.sh --system`.
- **Why it was dangerous**: System-wide daemons run as system services; injecting user-level Antigravity IDE configuration into root's home directory caused filesystem permission inconsistencies and config bloat.
- **Resolution**: Scoped MCP configuration registration strictly to `--user` mode so system installations only install systemd service units and coredump configs.

```bash
# Before:
register_mcp_config "${bin_dir}/killgravity-mcp" # executed in both user & system mode

# After:
if [ "$MODE" = "user" ]; then
    register_mcp_config "${bin_dir}/killgravity-mcp"
fi
```

---

### 2. Race Conditions in Test Process Lifecycle
- **Review**: [ID 13567](https://livereview.hexmos.com/#/reviews/13567) & [ID 13568](https://livereview.hexmos.com/#/reviews/13568)
- **File**: [`tests/test_detection.sh`](../../tests/test_detection.sh)
- **Flagged by `git-lrc`**: Static `sleep 0.5` and `sleep 1` intervals were used to wait for background mock processes to spawn and terminate. On loaded machines, this introduced non-deterministic test failures.
- **Resolution**: Replaced arbitrary static sleeps with active polling loops checking PID liveness up to 3 seconds in 100ms intervals.

```bash
# After:
# Active polling loop for process initialization
STARTED=false
for _ in {1..30}; do
    if kill -0 "$MOCK_PID" 2>/dev/null; then
        STARTED=true
        break
    fi
    sleep 0.1
done
```

---

### 3. Self-Termination via Loose Subprocess Fallback
- **Review**: [ID 13567](https://livereview.hexmos.com/#/reviews/13567) & Functional Smoke Testing
- **File**: [`bin/killgravity`](../../bin/killgravity)
- **Flagged by `git-lrc`**: A fallback `pkill -9 -f "$TARGET_PATTERN"` was executed after explicit PID killing. When running tests with `--pattern mock-antigravity-ide-test`, the `pkill -f` matched the caller command arguments, terminating the test runner itself with exit code 137.
- **Resolution**: Removed the wildcard `pkill -f` fallback. Process termination now exclusively uses verified, in-memory PID lists via `os.kill(pid, signal.SIGKILL)` with strict `my_pid` and `parent_pid` exclusions.

---

### 4. Missing Error Trapping in GitHub Actions CI
- **Review**: [ID 13570](https://livereview.hexmos.com/#/reviews/13570)
- **File**: [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml)
- **Flagged by `git-lrc`**: In multi-line `run: |` blocks, errors in intermediate commands would not halt the job if subsequent commands succeeded.
- **Resolution**: Explicitly added `set -euo pipefail` at the start of the installation test step.

```yaml
# Before:
- name: Test rootless installation
  run: |
    ./install.sh --user
    killgravity --version

# After:
- name: Test rootless installation
  run: |
    set -euo pipefail
    ./install.sh --user
    killgravity --version
```

---

### 5. Silent Swallowing of Malformed JSON-RPC Requests
- **Review**: [ID 13570](https://livereview.hexmos.com/#/reviews/13570)
- **File**: [`mcp/server.py`](../../mcp/server.py)
- **Flagged by `git-lrc`**: `mcp/server.py` caught `json.JSONDecodeError` and silently executed `continue`, leaving AI clients hanging without a response.
- **Resolution**: Added compliant JSON-RPC 2.0 error handling returning standard error code `-32700` (Parse error).

```python
# After:
except json.JSONDecodeError:
    err_resp = {
        "jsonrpc": "2.0",
        "id": None,
        "error": {
            "code": -32700,
            "message": "Parse error: Invalid JSON payload"
        }
    }
    sys.stdout.write(json.dumps(err_resp) + "\n")
    sys.stdout.flush()
    continue
```

---

### 6. Error Masking in Test Assertions
- **Review**: [ID 13567](https://livereview.hexmos.com/#/reviews/13567)
- **File**: [`tests/test_detection.sh`](../../tests/test_detection.sh)
- **Flagged by `git-lrc`**: Appending `|| true` to test execution calls masked non-zero exit codes from the command under test.
- **Resolution**: Removed `|| true` and directly asserted command success.

---

### 7. Blanket Systemd Error Suppression
- **Review**: [ID 13568](https://livereview.hexmos.com/#/reviews/13568)
- **File**: [`install.sh`](../../install.sh)
- **Flagged by `git-lrc`**: `systemctl daemon-reload 2>/dev/null || true` hid broken systemd states from the user.
- **Resolution**: Allowed standard systemctl errors to be surfaced so configuration issues can be diagnosed immediately.

---

### 8. Target Blast Radius & Process Discrimination
- **File**: [`bin/killgravity`](../../bin/killgravity) & [`config/killgravity.conf`](../../config/killgravity.conf)
- **Historical Analysis & Review**: Original shell implementations used generic patterns (`antigravity`), which inadvertently terminated `antigravity-hub`, `drun`, and background developer agents.
- **Resolution**: Scoped default pattern specifically to `antigravity-ide` and added dynamic path discovery for versioned directories (`/opt/antigravity-ide-*`).

---

### 9. Type Safety & Validation in Configuration Updates
- **File**: [`bin/killgravity`](../../bin/killgravity)
- **Issue**: `set_value()` directly assigned incoming CLI string arguments to `self.data[key]`, causing values like `POLL_INTERVAL="2"` or `AUTO_RESTART="false"` to be stored as strings instead of `int` or `bool`. Subsequent math and sleep calls could fail with TypeError.
- **Resolution**: Added strict type conversion and validation matching the `DEFAULTS` schema before writing to memory or disk.

---

### 10. Audit Syntax Check on Configuration File
- **File**: [`scripts/audit.sh`](../../scripts/audit.sh) & [`Makefile`](../../Makefile)
- **Issue**: `config/killgravity.conf` was included in the `bash -n` static syntax check array. Because it is a key-value configuration file and not an executable shell script, running `bash -n` was architecturally inappropriate.
- **Resolution**: Replaced `bash -n` on config files with a dedicated `KEY=VALUE` parser validation step.

---

### 11 & 12. Cross-Platform Portability with In-Place Updates (`sed -i`)
- **File**: [`scripts/release.sh`](../../scripts/release.sh)
- **Issue**: `sed -i` flags differ between GNU `sed` (Linux) and BSD `sed` (macOS/FreeBSD). BSD `sed` requires an empty extension string (e.g. `sed -i ''`), causing release scripts to fail or corrupt files on macOS.
- **Resolution**: Replaced `sed -i` invocations with standard library Python in-place updates, guaranteeing 100% cross-platform consistency.

---

## Conclusion

Automated reviews with `git-lrc` served as an effective pre-commit and pre-release safety net, catching subtle race conditions, CI error handling gaps, protocol compliance flaws, and system isolation bugs before code reached production.

