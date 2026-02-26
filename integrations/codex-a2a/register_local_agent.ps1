$ErrorActionPreference = "Stop"

$serviceBase = "http://127.0.0.1:8080"
$a2aBase = "http://127.0.0.1:9091"

curl -sS -X POST "$serviceBase/settings/a2a/save" `
  --data-urlencode "name=codex-local" `
  --data-urlencode "description=Local Codex CLI A2A wrapper" `
  --data-urlencode "endpoint=$a2aBase/a2a/rpc" `
  --data-urlencode "agent_card_url=$a2aBase/.well-known/agent-card.json" `
  --data-urlencode "enabled=on"

Write-Host "A2A agent registered. Verify with: curl -sS $serviceBase/api/a2a/agents"
