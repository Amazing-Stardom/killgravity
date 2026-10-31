#!/usr/bin/env python3
"""
killgravity-mcp: Model Context Protocol (MCP) server for killgravity.
Enables AI assistants (such as Antigravity) to monitor and control the disk guard,
adjust thresholds, manage service state, and execute emergency process terminations.

Zero-dependency standard library implementation (JSON-RPC 2.0 over stdio).
Copyright (c) 2026 Ganesh Kumar. Licensed under the MIT License.
"""

import sys
import json
import os
import subprocess

VERSION = "1.1.0"

# Locate killgravity binary
def get_killgravity_cmd():
    user_bin = os.path.expanduser("~/.local/bin/killgravity")
    if os.path.isfile(user_bin) and os.access(user_bin, os.X_OK):
        return user_bin
    
    # Check current repo bin directory
    repo_bin = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "bin", "killgravity"))
    if os.path.isfile(repo_bin) and os.access(repo_bin, os.X_OK):
        return repo_bin

    sys_bin = "/usr/local/bin/killgravity"
    if os.path.isfile(sys_bin) and os.access(sys_bin, os.X_OK):
        return sys_bin

    return "killgravity"

def run_killgravity(args):
    cmd = [get_killgravity_cmd()] + args
    try:
        res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=15)
        out = res.stdout
        if res.stderr:
            out += "\n" + res.stderr
        return {
            "success": res.returncode == 0,
            "output": out.strip()
        }
    except Exception as e:
        return {
            "success": False,
            "output": f"Execution error: {str(e)}"
        }

# Tool schemas
TOOLS = [
    {
        "name": "killgravity_status",
        "description": "Get live watchdog status including systemd service state, target Antigravity IDE processes, active write threshold, timings, and real-time NVMe disk write speed.",
        "inputSchema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    },
    {
        "name": "killgravity_threshold",
        "description": "View or adjust the disk write trigger threshold (e.g. '500M', '1G', or raw sectors). Automatically persists to config and reloads active service.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "threshold": {
                    "type": "string",
                    "description": "Optional new threshold (e.g. '500M', '1G', '1000000'). Leave empty to query current setting."
                }
            },
            "required": []
        }
    },
    {
        "name": "killgravity_service",
        "description": "Manage the killgravity background watchdog systemd service.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "action": {
                    "type": "string",
                    "enum": ["start", "stop", "restart", "enable", "disable", "logs"],
                    "description": "Action to perform on the service."
                }
            },
            "required": ["action"]
        }
    },
    {
        "name": "killgravity_timing",
        "description": "View or configure watchdog timing intervals (poll interval, post-kill cooldown, restart delay).",
        "inputSchema": {
            "type": "object",
            "properties": {
                "setting": {
                    "type": "string",
                    "enum": ["interval", "cooldown", "delay"],
                    "description": "Which timing parameter to adjust."
                },
                "seconds": {
                    "type": "integer",
                    "description": "Duration in seconds."
                }
            },
            "required": []
        }
    },
    {
        "name": "killgravity_emergency_kill",
        "description": "Immediately terminate runaway Antigravity IDE processes and purge partial core/crashpad dump files to rescue disk space and protect NVMe TBW.",
        "inputSchema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    },
    {
        "name": "killgravity_clean",
        "description": "Scan and purge stale crashpad dumps (.dmp) and core dump artifacts from temporary and user directories.",
        "inputSchema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    },
    {
        "name": "killgravity_test",
        "description": "Run diagnostic verification probe on /proc/diskstats read capability, current I/O write rate, and target process matching.",
        "inputSchema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    }
]

def handle_tool_call(name, arguments):
    if name == "killgravity_status":
        res = run_killgravity(["status"])
        return res["output"]

    elif name == "killgravity_threshold":
        threshold = arguments.get("threshold")
        args = ["threshold"]
        if threshold:
            args.append(str(threshold))
        res = run_killgravity(args)
        return res["output"]

    elif name == "killgravity_service":
        action = arguments.get("action")
        if action == "logs":
            res = run_killgravity(["logs", "-n", "30", "--no-pager"])
        else:
            res = run_killgravity([action])
        return res["output"]

    elif name == "killgravity_timing":
        setting = arguments.get("setting")
        seconds = arguments.get("seconds")
        if not setting:
            res = run_killgravity(["time"])
        else:
            args = ["time", setting]
            if seconds is not None:
                args.append(str(seconds))
            res = run_killgravity(args)
        return res["output"]

    elif name == "killgravity_emergency_kill":
        res = run_killgravity(["kill"])
        return res["output"]

    elif name == "killgravity_clean":
        res = run_killgravity(["clean"])
        return res["output"]

    elif name == "killgravity_test":
        res = run_killgravity(["test"])
        return res["output"]

    else:
        return f"Unknown tool: {name}"

def main():
    while True:
        line = sys.stdin.readline()
        if not line:
            break
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
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

        req_id = req.get("id")
        method = req.get("method")
        params = req.get("params", {})

        if method == "initialize":
            resp = {
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {
                        "tools": {}
                    },
                    "serverInfo": {
                        "name": "killgravity",
                        "version": VERSION
                    }
                }
            }
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()

        elif method == "notifications/initialized":
            pass

        elif method == "tools/list":
            resp = {
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {
                    "tools": TOOLS
                }
            }
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()

        elif method == "tools/call":
            tool_name = params.get("name")
            tool_args = params.get("arguments", {})
            result_text = handle_tool_call(tool_name, tool_args)
            resp = {
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {
                    "content": [
                        {
                            "type": "text",
                            "text": result_text
                        }
                    ]
                }
            }
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()

        elif method == "ping":
            resp = {
                "jsonrpc": "2.0",
                "id": req_id,
                "result": {}
            }
            sys.stdout.write(json.dumps(resp) + "\n")
            sys.stdout.flush()

        else:
            if req_id is not None:
                resp = {
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "error": {
                        "code": -32601,
                        "message": f"Method not found: {method}"
                    }
                }
                sys.stdout.write(json.dumps(resp) + "\n")
                sys.stdout.flush()

if __name__ == "__main__":
    main()
