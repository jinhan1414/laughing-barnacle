$ErrorActionPreference = "Stop"

$serviceBase = "http://127.0.0.1:8080"
$a2aBase = "http://127.0.0.1:9091"

$payload = @{
  name = "codex-local"
  description = "Local Codex CLI A2A agent powered by official a2a-python SDK"
  endpoint = "$a2aBase/a2a/rpc"
  agent_card_url = "$a2aBase/.well-known/agent-card.json"
  enabled = $true
} | ConvertTo-Json -Compress

curl -sS -X POST "$serviceBase/api/a2a/agents/save" `
  -H "Content-Type: application/json" `
  -d "$payload"

Write-Host "A2A agent registered. Verify with: curl -sS $serviceBase/api/a2a/agents"
