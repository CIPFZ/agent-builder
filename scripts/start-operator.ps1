param(
  [string]$DaemonPath = ".\myclawd.exe",
  [string]$HostName = "127.0.0.1",
  [int]$DaemonPort = 18080,
  [int]$WebPort = 5173,
  [switch]$OpenBrowser
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$operatorDir = Join-Path $root "web\operator"
$daemonCandidate = if ([System.IO.Path]::IsPathRooted($DaemonPath)) { $DaemonPath } else { Join-Path $root $DaemonPath }
$daemonFullPath = Resolve-Path -LiteralPath $daemonCandidate

function Stop-ProcessTree([int]$ProcessId) {
  $children = Get-CimInstance Win32_Process -Filter "ParentProcessId = $ProcessId" -ErrorAction SilentlyContinue
  foreach ($child in $children) {
    Stop-ProcessTree $child.ProcessId
  }
  Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
}

if (-not (Test-Path -LiteralPath (Join-Path $operatorDir "node_modules"))) {
  Write-Host "Installing Operator UI dependencies..."
  Push-Location $operatorDir
  try {
    npm install
  } finally {
    Pop-Location
  }
}

$daemonUrl = "http://${HostName}:${DaemonPort}"
$webUrl = "http://${HostName}:${WebPort}"
$wsUrl = "ws://${HostName}:${DaemonPort}/ws"

Write-Host "Starting myclawd: $daemonUrl"
$daemon = Start-Process -FilePath $daemonFullPath -WorkingDirectory $root -NoNewWindow -PassThru

Write-Host "Starting Operator UI dev server: $webUrl"
$web = Start-Process -FilePath "npm" -ArgumentList @("run", "dev", "--", "--host", $HostName, "--port", "$WebPort") -WorkingDirectory $operatorDir -NoNewWindow -PassThru

function Wait-HttpOk([string]$Url, [int]$TimeoutSeconds) {
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  do {
    try {
      $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2
      if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 500) {
        return
      }
    } catch {
      Start-Sleep -Milliseconds 500
    }
  } while ((Get-Date) -lt $deadline)
  throw "Timed out waiting for $Url"
}

try {
  Wait-HttpOk "$daemonUrl/healthz" 30
  Wait-HttpOk $webUrl 30

  Write-Host ""
  Write-Host "myclaw Operator is ready."
  Write-Host "Open: $webUrl"
  Write-Host "WebSocket: $wsUrl"
  Write-Host "Press Ctrl+C to stop both processes."

  if ($OpenBrowser) {
    Start-Process $webUrl | Out-Null
  }

  while (-not $daemon.HasExited -and -not $web.HasExited) {
    Start-Sleep -Seconds 1
  }
} finally {
  foreach ($process in @($web, $daemon)) {
    if ($process -and -not $process.HasExited) {
      Stop-ProcessTree $process.Id
    }
  }
}
