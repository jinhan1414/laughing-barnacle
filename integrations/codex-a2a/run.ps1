$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$projectRoot = Split-Path -Parent $root
$scriptPath = Join-Path $PSScriptRoot "codex_a2a_agent.py"
$stateFile = Join-Path $PSScriptRoot "state\tasks.json"

if (-not (Test-Path $stateFile)) {
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $stateFile) | Out-Null
  "{}" | Set-Content -Path $stateFile -Encoding UTF8
}

python $scriptPath --workdir $projectRoot --host 127.0.0.1 --port 9091 --state-file $stateFile
