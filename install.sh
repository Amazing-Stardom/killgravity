#!/usr/bin/env bash
#
# install.sh - Installer & Uninstaller for killgravity (CLI, Systemd, & MCP)
# Supports user-level (rootless) and system-wide installation/uninstallation.
#
# Copyright (c) 2026 Ganesh Kumar
# Licensed under the MIT License.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Formatting
if [ -t 1 ]; then
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    GREEN=''
    YELLOW=''
    BLUE=''
    BOLD=''
    NC=''
fi

ACTION="install"
if [[ "$(basename "$0")" == *"uninstall"* ]]; then
    ACTION="uninstall"
fi

MODE="auto"
PURGE=false

print_help() {
    cat <<EOF
killgravity installer & uninstaller

Usage:
  ./install.sh [ACTION] [OPTIONS]

Actions:
  install            Install killgravity (default)
  uninstall          Uninstall killgravity

Options:
  --user             Install/uninstall for current user only (rootless)
  --system           Install/uninstall system-wide (requires sudo / root)
  --uninstall        Shortcut to select uninstall action
  --purge            Remove configuration files as well (during uninstall)
  -h, --help         Show this help text

Examples:
  ./install.sh                       # Install in user mode (or system if root)
  ./install.sh --user                # Force user mode installation
  sudo ./install.sh --system         # Install system-wide with coredump suppression
  ./install.sh uninstall             # Uninstall user mode
  ./install.sh --uninstall --purge   # Uninstall and remove user configuration
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        install)
            ACTION="install"
            shift
            ;;
        uninstall|--uninstall)
            ACTION="uninstall"
            shift
            ;;
        --user)
            MODE="user"
            shift
            ;;
        --system)
            MODE="system"
            shift
            ;;
        --purge)
            PURGE=true
            shift
            ;;
        -h|--help)
            print_help
            exit 0
            ;;
        *)
            echo -e "${YELLOW}Unknown option or argument: $1${NC}"
            print_help
            exit 1
            ;;
    esac
done

if [ "$MODE" = "auto" ]; then
    if [ "$(id -u)" -eq 0 ]; then
        MODE="system"
    else
        MODE="user"
    fi
fi

register_mcp_config() {
    local mcp_bin="$1"
    local mcp_cfg="${HOME}/.gemini/config/mcp_config.json"

    if [ -d "${HOME}/.gemini/config" ]; then
        echo -e "${BLUE}->${NC} Registering killgravity MCP server in ${mcp_cfg}..."
        python3 - <<EOF
import json, os

cfg_path = os.path.expanduser("${mcp_cfg}")
mcp_bin = os.path.expanduser("${mcp_bin}")

data = {"mcpServers": {}}
if os.path.isfile(cfg_path):
    try:
        with open(cfg_path, "r") as f:
            data = json.load(f)
    except Exception:
        data = {"mcpServers": {}}

if "mcpServers" not in data:
    data["mcpServers"] = {}

data["mcpServers"]["killgravity"] = {
    "command": "python3",
    "args": [mcp_bin]
}

with open(cfg_path, "w") as f:
    json.dump(data, f, indent=2)

print("  ✓ Configured MCP entry for Antigravity.")
EOF
    fi
}

unregister_mcp_config() {
    local mcp_cfg="${HOME}/.gemini/config/mcp_config.json"
    if [ -f "$mcp_cfg" ]; then
        python3 - <<EOF
import json, os
cfg_path = os.path.expanduser("${mcp_cfg}")
try:
    with open(cfg_path, "r") as f:
        data = json.load(f)
    if "mcpServers" in data and "killgravity" in data["mcpServers"]:
        del data["mcpServers"]["killgravity"]
        with open(cfg_path, "w") as f:
            json.dump(data, f, indent=2)
        print("  ✓ Removed MCP entry from Antigravity config.")
except Exception:
    pass
EOF
    fi
}

# ==============================================================================
# INSTALL ACTION
# ==============================================================================
do_install() {
    echo -e "${BOLD}Installing killgravity in [${MODE}] mode...${NC}"

    if [ "$MODE" = "user" ]; then
        local bin_dir="${HOME}/.local/bin"
        local conf_dir="${HOME}/.config/killgravity"
        local systemd_dir="${HOME}/.config/systemd/user"

        mkdir -p "$bin_dir" "$conf_dir" "$systemd_dir"

        echo -e "${BLUE}->${NC} Installing CLI binary to ${bin_dir}/killgravity"
        install -m 755 "${SCRIPT_DIR}/bin/killgravity" "${bin_dir}/killgravity"

        echo -e "${BLUE}->${NC} Installing MCP server to ${bin_dir}/killgravity-mcp"
        install -m 755 "${SCRIPT_DIR}/mcp/server.py" "${bin_dir}/killgravity-mcp"

        if [ ! -f "${conf_dir}/killgravity.conf" ]; then
            echo -e "${BLUE}->${NC} Installing default config to ${conf_dir}/killgravity.conf"
            install -m 644 "${SCRIPT_DIR}/config/killgravity.conf" "${conf_dir}/killgravity.conf"
        else
            echo -e "${YELLOW}->${NC} Preserving existing config at ${conf_dir}/killgravity.conf"
        fi

        echo -e "${BLUE}->${NC} Installing systemd user unit to ${systemd_dir}/killgravity.service"
        install -m 644 "${SCRIPT_DIR}/systemd/killgravity.service" "${systemd_dir}/killgravity.service"

        if command -v systemctl >/dev/null 2>&1; then
            echo -e "${BLUE}->${NC} Reloading systemd user daemon..."
            systemctl --user daemon-reload 2>/dev/null || true
            echo -e "${BLUE}->${NC} Enabling and starting killgravity.service..."
            systemctl --user enable --now killgravity.service 2>/dev/null || true
        fi

        # Register MCP in Antigravity
        register_mcp_config "${bin_dir}/killgravity-mcp"

        # Install Antigravity IDE extension if IDE exists
        local ext_installed=false
        local ext_ver="1.0.0"
        if [ -f "${SCRIPT_DIR}/ide-extension/package.json" ]; then
            ext_ver=$(grep -E '"version"\s*:' "${SCRIPT_DIR}/ide-extension/package.json" | head -n1 | cut -d'"' -f4)
        fi
        local ext_dir="${HOME}/.antigravity-ide/extensions/hexmos.killgravity-${ext_ver}-universal"
        if [ -d "${HOME}/.antigravity-ide/extensions" ] || [ -d "${HOME}/.config/Antigravity IDE" ]; then
            echo -e "${BLUE}->${NC} Installing Antigravity IDE extension to ${ext_dir}"
            mkdir -p "$ext_dir"
            cp "${SCRIPT_DIR}/ide-extension/package.json" "${ext_dir}/"
            cp "${SCRIPT_DIR}/ide-extension/extension.js" "${ext_dir}/"
            ext_installed=true
        fi

        echo -e "\n${GREEN}${BOLD}✓ User installation complete!${NC}"
        echo -e "CLI Binary:  ${bin_dir}/killgravity"
        echo -e "MCP Server:  ${bin_dir}/killgravity-mcp"
        echo -e "Config:      ${conf_dir}/killgravity.conf"
        echo -e "Service:     killgravity.service (systemctl --user)"
        if [ "$ext_installed" = "true" ]; then
            echo -e "Antigravity: MCP tools & IDE extension enabled"
        else
            echo -e "Antigravity: MCP tools enabled (killgravity_*)"
        fi
        echo -e "\nVerify with:"
        echo -e "  killgravity status"

        if [[ ":$PATH:" != *":$bin_dir:"* ]]; then
            echo -e "\n${YELLOW}Note:${NC} ${bin_dir} is not in your current PATH. Add it to ~/.bashrc or ~/.zshrc:"
            echo -e "  export PATH=\"\$HOME/.local/bin:\$PATH\""
        fi

    else
        # System mode
        if [ "$(id -u)" -ne 0 ]; then
            echo -e "${YELLOW}Error: System-wide installation requires root privileges.${NC}" >&2
            echo "Run: sudo ./install.sh --system" >&2
            exit 1
        fi

        local bin_dir="/usr/local/bin"
        local conf_dir="/etc/killgravity"
        local systemd_dir="/etc/systemd/system"
        local coredump_dir="/etc/systemd/coredump.conf.d"

        mkdir -p "$bin_dir" "$conf_dir" "$systemd_dir" "$coredump_dir"

        echo -e "${BLUE}->${NC} Installing CLI binary to ${bin_dir}/killgravity"
        install -m 755 "${SCRIPT_DIR}/bin/killgravity" "${bin_dir}/killgravity"

        echo -e "${BLUE}->${NC} Installing MCP server to ${bin_dir}/killgravity-mcp"
        install -m 755 "${SCRIPT_DIR}/mcp/server.py" "${bin_dir}/killgravity-mcp"

        if [ ! -f "${conf_dir}/killgravity.conf" ]; then
            echo -e "${BLUE}->${NC} Installing default config to ${conf_dir}/killgravity.conf"
            install -m 644 "${SCRIPT_DIR}/config/killgravity.conf" "${conf_dir}/killgravity.conf"
        else
            echo -e "${YELLOW}->${NC} Preserving existing config at ${conf_dir}/killgravity.conf"
        fi

        echo -e "${BLUE}->${NC} Installing systemd system unit to ${systemd_dir}/killgravity.service"
        install -m 644 "${SCRIPT_DIR}/systemd/killgravity-system.service" "${systemd_dir}/killgravity.service"

        echo -e "${BLUE}->${NC} Installing systemd coredump drop-in to ${coredump_dir}/99-killgravity.conf"
        install -m 644 "${SCRIPT_DIR}/systemd/99-killgravity.conf" "${coredump_dir}/99-killgravity.conf"

        if command -v systemctl >/dev/null 2>&1; then
            echo -e "${BLUE}->${NC} Reloading systemd daemon..."
            systemctl daemon-reload
            echo -e "${BLUE}->${NC} Enabling and starting killgravity.service..."
            systemctl enable --now killgravity.service
        fi

        echo -e "\n${GREEN}${BOLD}✓ System installation complete!${NC}"
        echo -e "CLI Binary: ${bin_dir}/killgravity"
        echo -e "MCP Server: ${bin_dir}/killgravity-mcp"
        echo -e "Config:     ${conf_dir}/killgravity.conf"
        echo -e "Service:    killgravity.service (systemctl status killgravity)"
        echo -e "Coredump:   ${coredump_dir}/99-killgravity.conf"
    fi
}

# ==============================================================================
# UNINSTALL ACTION
# ==============================================================================
do_uninstall() {
    echo -e "${BOLD}Uninstalling killgravity [${MODE}] mode...${NC}"

    if [ "$MODE" = "user" ]; then
        if command -v systemctl >/dev/null 2>&1; then
            echo -e "${BLUE}->${NC} Stopping and disabling user service..."
            systemctl --user stop killgravity.service 2>/dev/null || true
            systemctl --user disable killgravity.service 2>/dev/null || true
        fi

        echo -e "${BLUE}->${NC} Removing files..."
        rm -f "${HOME}/.local/bin/killgravity"
        rm -f "${HOME}/.local/bin/killgravity-mcp"
        rm -f "${HOME}/.config/systemd/user/killgravity.service"
        rm -rf "${HOME}/.antigravity-ide/extensions/hexmos.killgravity-"*

        unregister_mcp_config

        if [ "$PURGE" = "true" ]; then
            rm -rf "${HOME}/.config/killgravity"
            echo -e "${BLUE}->${NC} Configuration purged."
        else
            echo -e "${YELLOW}->${NC} Kept configuration at ${HOME}/.config/killgravity (pass --purge to remove)."
        fi

        if command -v systemctl >/dev/null 2>&1; then
            systemctl --user daemon-reload 2>/dev/null || true
        fi

        echo -e "\n${GREEN}${BOLD}✓ User uninstallation complete.${NC}"

    else
        # System mode
        if [ "$(id -u)" -ne 0 ]; then
            echo -e "${YELLOW}Error: System-wide uninstallation requires root privileges.${NC}" >&2
            echo "Run: sudo ./install.sh uninstall --system" >&2
            exit 1
        fi

        if command -v systemctl >/dev/null 2>&1; then
            echo -e "${BLUE}->${NC} Stopping and disabling system service..."
            systemctl stop killgravity.service 2>/dev/null || true
            systemctl disable killgravity.service 2>/dev/null || true
        fi

        echo -e "${BLUE}->${NC} Removing files..."
        rm -f "/usr/local/bin/killgravity"
        rm -f "/usr/local/bin/killgravity-mcp"
        rm -f "/etc/systemd/system/killgravity.service"
        rm -f "/etc/systemd/coredump.conf.d/99-killgravity.conf"

        unregister_mcp_config

        if [ "$PURGE" = "true" ]; then
            rm -rf "/etc/killgravity"
            echo -e "${BLUE}->${NC} Configuration purged."
        else
            echo -e "${YELLOW}->${NC} Kept configuration at /etc/killgravity (pass --purge to remove)."
        fi

        if command -v systemctl >/dev/null 2>&1; then
            systemctl daemon-reload 2>/dev/null || true
        fi

        echo -e "\n${GREEN}${BOLD}✓ System uninstallation complete.${NC}"
    fi
}

case "$ACTION" in
    install)
        do_install
        ;;
    uninstall)
        do_uninstall
        ;;
esac
