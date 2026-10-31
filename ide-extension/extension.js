const vscode = require('vscode');
const { execFile } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

let statusBarItem;
let pollTimer;

/**
 * Resolves the killgravity executable from configuration, standard install paths, or PATH.
 */
function resolveKillgravityBin() {
  const cfg = vscode.workspace.getConfiguration('killgravity');
  const custom = cfg.get('executablePath');
  if (custom && fs.existsSync(custom)) {
    return custom;
  }

  const userBin = path.join(os.homedir(), '.local', 'bin', 'killgravity');
  if (fs.existsSync(userBin)) {
    return userBin;
  }

  const sysBin = '/usr/local/bin/killgravity';
  if (fs.existsSync(sysBin)) {
    return sysBin;
  }

  // Check workspace bin if working inside repo
  if (vscode.workspace.workspaceFolders && vscode.workspace.workspaceFolders.length > 0) {
    for (const folder of vscode.workspace.workspaceFolders) {
      const repoBin = path.join(folder.uri.fsPath, 'bin', 'killgravity');
      if (fs.existsSync(repoBin)) {
        return repoBin;
      }
    }
  }

  return 'killgravity';
}

/**
 * Executes a killgravity subcommand with arguments.
 */
function runKillgravity(args) {
  return new Promise((resolve, reject) => {
    const bin = resolveKillgravityBin();
    execFile(bin, args, { timeout: 10000 }, (err, stdout, stderr) => {
      if (err) {
        return reject(new Error(stderr || stdout || err.message));
      }
      resolve(stdout.trim());
    });
  });
}

/**
 * Queries the main script for live JSON metrics.
 */
async function fetchLiveSample() {
  try {
    const raw = await runKillgravity(['sample', '--json']);
    return JSON.parse(raw);
  } catch (e) {
    return null;
  }
}

/**
 * Queries the main script for full status JSON.
 */
async function fetchFullStatus() {
  try {
    const raw = await runKillgravity(['status', '--json']);
    return JSON.parse(raw);
  } catch (e) {
    return null;
  }
}

/**
 * Updates the Status Bar indicator with latest data from the main script.
 */
async function updateStatus() {
  if (!statusBarItem) return;

  const data = await fetchLiveSample();
  if (!data) {
    statusBarItem.text = '$(database) killgravity: off';
    statusBarItem.tooltip = 'killgravity binary not reachable. Click to configure.';
    statusBarItem.backgroundColor = undefined;
    statusBarItem.show();
    return;
  }

  const speedMb = data.write_mb_per_sec || 0;
  const sectors = data.write_sectors_per_sec || 0;
  const threshMb = data.threshold_mb || 488;
  const isActive = data.service_active;
  const pids = data.target_count || 0;

  if (data.surge_detected) {
    statusBarItem.text = `$(error) ${speedMb} MB/s SURGE!`;
    statusBarItem.tooltip = `CRITICAL: Write speed (${speedMb} MB/s) exceeds threshold (${threshMb} MB/s)!\nAntigravity crash dump loop detected.\nClick to inspect or kill.`;
    statusBarItem.backgroundColor = new vscode.ThemeColor('statusBarItem.errorBackground');
  } else if (speedMb > 150) {
    statusBarItem.text = `$(alert) ${speedMb} MB/s`;
    statusBarItem.tooltip = `Elevated Disk Writes: ${speedMb} MB/s (${sectors} sectors/s)\nThreshold: ${threshMb} MB/s\nGuard: ${isActive ? 'RUNNING' : 'STOPPED'}\nClick for options.`;
    statusBarItem.backgroundColor = new vscode.ThemeColor('statusBarItem.warningBackground');
  } else {
    const icon = isActive ? '$(shield)' : '$(database)';
    statusBarItem.text = `${icon} ${speedMb} MB/s`;
    statusBarItem.tooltip = `killgravity Disk Guard\nStatus: ${isActive ? 'ACTIVE (Protected)' : 'STOPPED'}\nWrite Rate: ${speedMb} MB/s (${sectors} sectors/s)\nLimit: ${threshMb} MB/s\nActive PIDs: ${pids}\nClick for controls.`;
    statusBarItem.backgroundColor = undefined;
  }

  statusBarItem.show();
}

/**
 * Shows an interactive QuickPick menu directly reflecting the main script state.
 */
async function showStatusMenu() {
  const status = await fetchFullStatus();
  if (!status) {
    const choice = await vscode.window.showErrorMessage(
      'Could not communicate with killgravity main script. Make sure killgravity is installed.',
      'Locate Binary'
    );
    if (choice === 'Locate Binary') {
      const uris = await vscode.window.showOpenDialog({ canSelectFiles: true, canSelectFolders: false, title: 'Select killgravity executable' });
      if (uris && uris[0]) {
        await vscode.workspace.getConfiguration('killgravity').update('executablePath', uris[0].fsPath, vscode.ConfigurationTarget.Global);
        vscode.window.showInformationMessage(`Updated killgravity path to: ${uris[0].fsPath}`);
        updateStatus();
      }
    }
    return;
  }

  const isRunning = status.service.active;
  const items = [
    {
      label: isRunning ? '$(stop) Stop Guard Service' : '$(play) Start Guard Service',
      description: isRunning ? 'Currently RUNNING' : 'Currently STOPPED',
      detail: isRunning ? 'Click to pause background watchdog' : 'Click to activate background watchdog protection',
      action: isRunning ? 'stop' : 'start'
    },
    {
      label: `$(dashboard) Disk Write Rate: ${status.io.write_mb_per_sec} MB/s`,
      description: `Limit: ${status.io.threshold_mb_per_sec} MB/s (${status.io.headroom_pct}% headroom)`,
      detail: `Sample: ${status.io.write_sectors_per_sec} sectors/s | Surge status: ${status.io.surge_detected ? 'DETECTED' : 'None'}`,
      action: 'refresh'
    },
    {
      label: `$(pulse) Monitored Processes: ${status.target.count} PIDs active`,
      description: `Pattern: ${status.target.pattern}`,
      detail: `PIDs: ${status.target.pids.slice(0, 10).join(', ')}${status.target.pids.length > 10 ? '...' : ''}`,
      action: 'refresh'
    },
    {
      label: '$(trash) Clean Crash & Core Dump Files',
      description: 'Purge stale .dmp and core dumps',
      detail: 'Frees disk space from past Antigravity IDE crashpad dumps',
      action: 'clean'
    },
    {
      label: '$(flame) Emergency Kill Runaway Processes',
      description: 'Terminate all target antigravity-ide processes',
      detail: 'Instantly stops crash loops if disk write surge is running wild',
      action: 'kill'
    }
  ];

  const pick = await vscode.window.showQuickPick(items, {
    placeHolder: `killgravity v${status.version} • Guard: ${status.service.status} • ${status.io.write_mb_per_sec} MB/s`
  });

  if (!pick) return;

  if (pick.action === 'start') {
    try {
      await runKillgravity(['start']);
      vscode.window.showInformationMessage('killgravity guard service started.');
    } catch (e) {
      vscode.window.showErrorMessage(`Failed to start service: ${e.message}`);
    }
    updateStatus();
  } else if (pick.action === 'stop') {
    try {
      await runKillgravity(['stop']);
      vscode.window.showInformationMessage('killgravity guard service stopped.');
    } catch (e) {
      vscode.window.showErrorMessage(`Failed to stop service: ${e.message}`);
    }
    updateStatus();
  } else if (pick.action === 'clean') {
    try {
      const out = await runKillgravity(['clean']);
      vscode.window.showInformationMessage(out);
    } catch (e) {
      vscode.window.showErrorMessage(`Failed to clean dumps: ${e.message}`);
    }
  } else if (pick.action === 'kill') {
    const confirm = await vscode.window.showWarningMessage(
      'Are you sure you want to kill all running Antigravity IDE processes? This will close your editor session.',
      'Yes, Terminate', 'Cancel'
    );
    if (confirm === 'Yes, Terminate') {
      try {
        await runKillgravity(['kill']);
      } catch (e) {
        vscode.window.showErrorMessage(`Kill failed: ${e.message}`);
      }
    }
  } else {
    updateStatus();
  }
}

function activate(context) {
  // Create status bar item on the right side
  statusBarItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
  statusBarItem.command = 'killgravity.status';
  context.subscriptions.push(statusBarItem);

  // Register commands
  context.subscriptions.push(
    vscode.commands.registerCommand('killgravity.status', showStatusMenu),
    vscode.commands.registerCommand('killgravity.start', async () => {
      await runKillgravity(['start']);
      vscode.window.showInformationMessage('killgravity started.');
      updateStatus();
    }),
    vscode.commands.registerCommand('killgravity.stop', async () => {
      await runKillgravity(['stop']);
      vscode.window.showInformationMessage('killgravity stopped.');
      updateStatus();
    }),
    vscode.commands.registerCommand('killgravity.clean', async () => {
      const out = await runKillgravity(['clean']);
      vscode.window.showInformationMessage(out);
    }),
    vscode.commands.registerCommand('killgravity.emergencyKill', async () => {
      await runKillgravity(['kill']);
      vscode.window.showInformationMessage('Runaway targets terminated.');
    })
  );

  // Initial read and polling loop
  updateStatus();

  const cfg = vscode.workspace.getConfiguration('killgravity');
  const intervalSec = Math.max(1, cfg.get('pollIntervalSeconds') || 2);
  pollTimer = setInterval(updateStatus, intervalSec * 1000);

  context.subscriptions.push({
    dispose: () => {
      if (pollTimer) clearInterval(pollTimer);
    }
  });
}

function deactivate() {
  if (pollTimer) {
    clearInterval(pollTimer);
  }
}

module.exports = {
  activate,
  deactivate
};

