param(
  [switch]$Build
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$desktopRoot = Resolve-Path (Join-Path $scriptDir "..")
$repoRoot = Resolve-Path (Join-Path $desktopRoot "..")
$exePath = Join-Path $desktopRoot "bin\AgentBuilder.exe"
$runtimeDevRoot = Join-Path $repoRoot "tmp\runtime-dev"
$smokeRoot = Join-Path $runtimeDevRoot ("phase62-wails-packaged-smoke-" + [System.Guid]::NewGuid().ToString("N"))

function Run($command, $arguments, $workingDirectory) {
  Write-Host ">> $command $($arguments -join ' ')"
  $process = Start-Process -FilePath $command -ArgumentList $arguments -WorkingDirectory $workingDirectory -NoNewWindow -Wait -PassThru
  if ($process.ExitCode -ne 0) {
    throw "$command exited with code $($process.ExitCode)"
  }
}

try {
  New-Item -ItemType Directory -Path $smokeRoot -Force | Out-Null
  $env:AGENT_BUILDER_DESKTOP_ROOT = $smokeRoot

  if ($Build -or -not (Test-Path $exePath)) {
    Run "wails3" @("task", "sync:frontend") $desktopRoot
    Run "wails3" @("task", "build") $desktopRoot
  }

  if (-not (Test-Path $exePath)) {
    throw "AgentBuilder.exe was not found at $exePath"
  }

  Write-Host ">> go test . -run TestRuntimeBridgePhase62PackagedHandoffRecoveryContract"
  Run "go" @("test", ".", "-run", "TestRuntimeBridgePhase62PackagedHandoffRecoveryContract", "-count=1") $desktopRoot

  $exeProcess = Start-Process -FilePath $exePath -WorkingDirectory (Split-Path -Parent $exePath) -PassThru -WindowStyle Hidden
  Start-Sleep -Seconds 5
  if ($exeProcess.HasExited) {
    throw "AgentBuilder.exe exited during startup with code $($exeProcess.ExitCode)"
  }
  Stop-Process -Id $exeProcess.Id -Force
  Wait-Process -Id $exeProcess.Id -Timeout 10 -ErrorAction SilentlyContinue

  foreach ($dir in @("config", "data", "logs")) {
    $path = Join-Path $smokeRoot $dir
    if (-not (Test-Path $path)) {
      throw "Expected packaged runtime directory was not created: $path"
    }
  }

  Write-Host "Phase 6.2 Wails packaged smoke passed. Runtime root: $smokeRoot"
} finally {
  Remove-Item Env:\AGENT_BUILDER_DESKTOP_ROOT -ErrorAction SilentlyContinue
}
