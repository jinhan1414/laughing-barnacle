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

if (-not (Test-Path $stateFile)) {
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $stateFile) | Out-Null
  "{}" | Set-Content -Path $stateFile -Encoding UTF8
}

$codexBin = Resolve-CodexBin
python $scriptPath --workdir $projectRoot --host 127.0.0.1 --port 9091 --state-file $stateFile --codex-bin $codexBin
