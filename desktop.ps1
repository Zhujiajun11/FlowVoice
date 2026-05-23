param(
  [int]$Port = 4173
)

$env:FLOWVOICE_PORT = "$Port"
Set-Location $PSScriptRoot

Write-Host "FlowVoice 桌面模式启动中: http://localhost:$Port/"
go run . -mode desktop -open
