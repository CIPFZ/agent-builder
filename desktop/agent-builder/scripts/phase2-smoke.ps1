param(
  [switch]$Build,
  [switch]$Live
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$desktopRoot = Resolve-Path (Join-Path $scriptDir "..")
$repoRoot = Resolve-Path (Join-Path $desktopRoot "..\..")
$exePath = Join-Path $desktopRoot "bin\AgentBuilder.exe"
$smokeRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("agent-builder-phase2-smoke-" + [System.Guid]::NewGuid().ToString("N"))

function Run($command, $arguments, $workingDirectory) {
  Write-Host ">> $command $($arguments -join ' ')"
  $process = Start-Process -FilePath $command -ArgumentList $arguments -WorkingDirectory $workingDirectory -NoNewWindow -Wait -PassThru
  if ($process.ExitCode -ne 0) {
    throw "$command exited with code $($process.ExitCode)"
  }
}

function Get-OpenAICompatibleModels($baseURL, $apiKey) {
  $endpoint = $baseURL.TrimEnd("/") + "/v1/models"
  $response = Invoke-RestMethod -Method Get -Uri $endpoint -Headers @{
    Authorization = "Bearer $apiKey"
  } -TimeoutSec 30
  $models = @($response.data | ForEach-Object { $_.id } | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
  if ($models.Count -eq 0) {
    throw "Live smoke model discovery returned no models."
  }
  return $models
}

try {
  New-Item -ItemType Directory -Path $smokeRoot | Out-Null
  $env:AGENT_BUILDER_DESKTOP_ROOT = $smokeRoot

  if ($Build -or -not (Test-Path $exePath)) {
    Run "wails3" @("task", "sync:frontend") $desktopRoot
    Run "wails3" @("task", "build") $desktopRoot
  }

  if (-not (Test-Path $exePath)) {
    throw "AgentBuilder.exe was not found at $exePath"
  }

  if ($Live) {
    $providerKey = $env:DEEPSEEK_API_KEY
    if ([string]::IsNullOrWhiteSpace($providerKey)) {
      throw "Live smoke requires DEEPSEEK_API_KEY in the environment."
    }

    $models = Get-OpenAICompatibleModels "https://api.deepseek.com" $providerKey
    $selectedModel = $models[0]
    Write-Host "Live smoke selected model: $selectedModel"
    $modelConfig = @{
      protocol = "openai"
      url = "https://api.deepseek.com"
      apiKey = $providerKey
      model = $selectedModel
      models = $models
    } | ConvertTo-Json -Depth 4
    New-Item -ItemType Directory -Path (Join-Path $smokeRoot "config") -Force | Out-Null
    [System.IO.File]::WriteAllText(
      (Join-Path $smokeRoot "config\model.json"),
      $modelConfig + [Environment]::NewLine,
      [System.Text.UTF8Encoding]::new($false)
    )
  }

  Write-Host ">> go test ."
  Run "go" @("test", ".") $desktopRoot

  Write-Host ">> go test ./internal/db ./internal/runtimeapi"
  Run "go" @("test", "./internal/db", "./internal/runtimeapi") $repoRoot

  Write-Host ">> go test -run TestRuntimeServiceAPIEndpointBindsLoopbackWithToken"
  Run "go" @("test", ".", "-run", "TestRuntimeServiceAPIEndpointBindsLoopbackWithToken", "-count=1") $desktopRoot

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
      throw "Expected runtime directory was not created: $path"
    }
  }

  if ($Live) {
    Write-Host ">> go test -tags desktop_live . -run TestDesktopRuntimeBridgeLiveChat"
    Run "go" @("test", "-tags", "desktop_live", ".", "-run", "TestDesktopRuntimeBridgeLiveChat", "-count=1", "-timeout", "5m") $desktopRoot
  } else {
    Write-Host "Live provider smoke skipped. Set DEEPSEEK_API_KEY and pass -Live to run it."
  }

  Write-Host "Phase 2 smoke passed. Runtime root: $smokeRoot"
} finally {
  Remove-Item Env:\AGENT_BUILDER_DESKTOP_ROOT -ErrorAction SilentlyContinue
  if (Test-Path $smokeRoot) {
    Remove-Item -LiteralPath $smokeRoot -Recurse -Force -ErrorAction SilentlyContinue
  }
}
