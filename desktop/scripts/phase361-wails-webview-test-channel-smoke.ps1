param(
  [switch]$Build
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$desktopRoot = Resolve-Path (Join-Path $scriptDir "..")
$repoRoot = Resolve-Path (Join-Path $desktopRoot "..")
$exePath = Join-Path $desktopRoot "bin\AgentBuilder.exe"
$runtimeDevRoot = Join-Path $repoRoot "tmp\runtime-dev"
$smokeRoot = Join-Path $runtimeDevRoot ("phase361-wails-webview-test-channel-" + [System.Guid]::NewGuid().ToString("N"))
$webviewUserData = Join-Path $smokeRoot "webview-user-data"

function Run($command, $arguments, $workingDirectory) {
  Write-Host ">> $command $($arguments -join ' ')"
  $process = Start-Process -FilePath $command -ArgumentList $arguments -WorkingDirectory $workingDirectory -NoNewWindow -Wait -PassThru
  if ($process.ExitCode -ne 0) {
    throw "$command exited with code $($process.ExitCode)"
  }
}

function Get-FreeTcpPort() {
  $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
  try {
    $listener.Start()
    return $listener.LocalEndpoint.Port
  } finally {
    $listener.Stop()
  }
}

function WaitForRemoteDebugEndpoint($port) {
  $deadline = (Get-Date).AddSeconds(20)
  $url = "http://127.0.0.1:$port/json/version"
  while ((Get-Date) -lt $deadline) {
    try {
      $response = Invoke-WebRequest -UseBasicParsing -Uri $url -TimeoutSec 2
      if ($response.StatusCode -eq 200 -and $response.Content -match "webSocketDebuggerUrl") {
        return
      }
    } catch {
      Start-Sleep -Milliseconds 500
    }
  }
  throw "WebView2 remote debugging endpoint did not become ready at $url"
}

try {
  New-Item -ItemType Directory -Path $smokeRoot -Force | Out-Null
  New-Item -ItemType Directory -Path $webviewUserData -Force | Out-Null
  Set-Content -Path (Join-Path $smokeRoot "smoke.txt") -Value "Phase 36.1 Wails WebView test channel smoke" -Encoding UTF8

  Write-Host ">> go test -tags webview_test . -run TestDesktopWebviewTestOptions"
  Run "go" @("test", "-tags", "webview_test", ".", "-run", "TestDesktopWebviewTestOptions", "-count=1") $desktopRoot

  if ($Build -or -not (Test-Path $exePath)) {
    Run "wails3" @("task", "sync:frontend") $desktopRoot
    Run "wails3" @("task", "build", "EXTRA_TAGS=webview_test") $desktopRoot
  }

  if (-not (Test-Path $exePath)) {
    throw "AgentBuilder.exe was not found at $exePath"
  }

  $env:AGENT_BUILDER_DESKTOP_ROOT = $smokeRoot
  $remoteDebugPort = Get-FreeTcpPort
  $env:AGENT_BUILDER_WEBVIEW_TEST_REMOTE_DEBUG_PORT = "$remoteDebugPort"
  $env:AGENT_BUILDER_WEBVIEW_TEST_USER_DATA_DIR = $webviewUserData

  $exeProcess = Start-Process -FilePath $exePath -WorkingDirectory (Split-Path -Parent $exePath) -PassThru -WindowStyle Hidden
  Set-Content -Path (Join-Path $smokeRoot "AgentBuilder.pid") -Value "$($exeProcess.Id)" -Encoding UTF8
  Start-Sleep -Seconds 3
  if ($exeProcess.HasExited) {
    throw "AgentBuilder.exe exited during startup with code $($exeProcess.ExitCode)"
  }

  WaitForRemoteDebugEndpoint $remoteDebugPort

  foreach ($dir in @("config", "data", "logs", "webview-user-data")) {
    $path = Join-Path $smokeRoot $dir
    if (-not (Test-Path $path)) {
      throw "Expected packaged runtime directory was not created: $path"
    }
  }

  Write-Host "Phase 36.1 Wails WebView test channel smoke passed. Runtime root: $smokeRoot"
} finally {
  if ($exeProcess -and -not $exeProcess.HasExited) {
    Stop-Process -Id $exeProcess.Id -Force
    Wait-Process -Id $exeProcess.Id -Timeout 10 -ErrorAction SilentlyContinue
  }
  Remove-Item Env:\AGENT_BUILDER_DESKTOP_ROOT -ErrorAction SilentlyContinue
  Remove-Item Env:\AGENT_BUILDER_WEBVIEW_TEST_REMOTE_DEBUG_PORT -ErrorAction SilentlyContinue
  Remove-Item Env:\AGENT_BUILDER_WEBVIEW_TEST_USER_DATA_DIR -ErrorAction SilentlyContinue
}
