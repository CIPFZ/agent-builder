param(
  [ValidateSet('fresh-quiet', 'active-conversation', 'long-session', 'long-stream', 'recovery')]
  [string]$Scenario = 'fresh-quiet',
  [string]$ExecutablePath = '',
  [int]$DurationSeconds = -1,
  [double]$IntervalSeconds = 2,
  [double]$MaxPrivateMB = 0,
  [double]$MaxWorkingSetMB = 0,
  [double]$MaxRendererPrivateMB = 800,
  [int]$RendererSustainedSamples = 3
)

$ErrorActionPreference = 'Stop'

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if (-not $ExecutablePath) {
  $ExecutablePath = Join-Path $repoRoot 'desktop\bin\AgentBuilder.exe'
}
if (-not (Test-Path -LiteralPath $ExecutablePath -PathType Leaf)) {
  throw "Agent Builder executable was not found: $ExecutablePath"
}

$acceptanceRoot = Join-Path $repoRoot ("tmp\runtime-dev\memory-$Scenario-" + [guid]::NewGuid().ToString('N'))
$webviewUserData = Join-Path $acceptanceRoot 'webview-user-data'
$reportPath = Join-Path $acceptanceRoot "$Scenario-memory.json"
New-Item -ItemType Directory -Path $webviewUserData -Force | Out-Null

$listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
try {
  $listener.Start()
  $remoteDebugPort = $listener.LocalEndpoint.Port
} finally {
  $listener.Stop()
}

$env:AGENT_BUILDER_DESKTOP_ROOT = $acceptanceRoot
$env:AGENT_BUILDER_WEBVIEW_TEST_REMOTE_DEBUG_PORT = "$remoteDebugPort"
$env:AGENT_BUILDER_WEBVIEW_TEST_USER_DATA_DIR = $webviewUserData

$process = $null
try {
  $process = Start-Process `
    -FilePath $ExecutablePath `
    -WorkingDirectory (Split-Path -Parent $ExecutablePath) `
    -WindowStyle Hidden `
    -PassThru
  Start-Sleep -Seconds 5
  if ($process.HasExited) {
    throw "AgentBuilder exited during startup with code $($process.ExitCode)"
  }

  $profileArgs = @{
    ProcessName = 'AgentBuilder'
    RootProcessId = $process.Id
    Scenario = $Scenario
    IntervalSeconds = $IntervalSeconds
    MaxRendererPrivateMB = $MaxRendererPrivateMB
    RendererSustainedSamples = $RendererSustainedSamples
    OutputPath = $reportPath
    NoJsonOutput = $true
  }
  if ($DurationSeconds -ge 0) { $profileArgs.DurationSeconds = $DurationSeconds }
  if ($MaxPrivateMB -gt 0) { $profileArgs.MaxPrivateMB = $MaxPrivateMB }
  if ($MaxWorkingSetMB -gt 0) { $profileArgs.MaxWorkingSetMB = $MaxWorkingSetMB }

  & (Join-Path $PSScriptRoot 'conversation-memory-profile.ps1') @profileArgs
  Write-Host "ACCEPTANCE_REPORT=$reportPath"
} finally {
  if ($process -and -not $process.HasExited) {
    Stop-Process -Id $process.Id -Force
    Wait-Process -Id $process.Id -Timeout 10 -ErrorAction SilentlyContinue
  }
  Remove-Item Env:\AGENT_BUILDER_DESKTOP_ROOT -ErrorAction SilentlyContinue
  Remove-Item Env:\AGENT_BUILDER_WEBVIEW_TEST_REMOTE_DEBUG_PORT -ErrorAction SilentlyContinue
  Remove-Item Env:\AGENT_BUILDER_WEBVIEW_TEST_USER_DATA_DIR -ErrorAction SilentlyContinue
}
