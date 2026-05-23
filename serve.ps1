param(
  [int]$Port = 4173
)

$env:FLOWVOICE_PORT = "$Port"
Set-Location $PSScriptRoot

Write-Host "FlowVoice Go 服务启动中: http://localhost:$Port/"
go run .
