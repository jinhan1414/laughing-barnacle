$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$projectRoot = Split-Path -Parent $root
$scriptPath = Join-Path $PSScriptRoot "codex_a2a_agent.py"
$stateFile = Join-Path $PSScriptRoot "state\tasks.json"

function Resolve-CodexBin {
  $candidates = @("codex.cmd", "codex.exe", "codex")
  foreach ($candidate in $candidates) {
    $cmd = Get-Command $candidate -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Path -and (Test-Path $cmd.Path)) {
      return $cmd.Path
    }
  }
  throw "codex cli not found. Please ensure codex.cmd or codex.exe is available."
}

function Get-ListeningProcessIdsByPort {
  param([int]$Port)
  $rows = netstat -ano | findstr ":$Port" | findstr "LISTENING"
  $ids = @()
  foreach ($row in $rows) {
    $line = if ($row -is [string]) { $row } else { $row.Line }
    $text = ($line -replace "^\s+", "") -split "\s+"
    if ($text.Length -lt 5) { continue }
    $ids += $text[-1]
  }
  return $ids | Sort-Object -Unique
}

function Stop-StaleCodexA2AProcesses {
  param([int]$Port, [string]$ScriptPath)
  $normalizedScriptPath = [System.IO.Path]::GetFullPath($ScriptPath).ToLowerInvariant().Replace("/", "\")
  $procIds = Get-ListeningProcessIdsByPort -Port $Port
  if (-not $procIds -or $procIds.Count -eq 0) { return }
  foreach ($procId in $procIds) {
    $proc = Get-CimInstance Win32_Process -Filter "ProcessId=$procId" -ErrorAction SilentlyContinue
    if (-not $proc -or -not $proc.CommandLine) { continue }
    $cmd = $proc.CommandLine.ToLowerInvariant().Replace("/", "\")
    if ($cmd -notlike "*codex_a2a_agent.py*" -or $cmd -notlike "*$normalizedScriptPath*") { continue }
    Write-Host "Stopping stale codex-a2a process PID=$procId"
    Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
  }
}

if (-not (Test-Path $stateFile)) {
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $stateFile) | Out-Null
  "{}" | Set-Content -Path $stateFile -Encoding UTF8
}

Stop-StaleCodexA2AProcesses -Port 9091 -ScriptPath $scriptPath
$remaining = Get-ListeningProcessIdsByPort -Port 9091
if ($remaining -and $remaining.Count -gt 0) {
  throw "port 9091 is still occupied by PID(s): $($remaining -join ', ')"
}
$codexBin = Resolve-CodexBin
python $scriptPath --workdir $projectRoot --host 127.0.0.1 --port 9091 --state-file $stateFile --codex-bin $codexBin
