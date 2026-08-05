$ErrorActionPreference = 'Stop'

$profileScript = Join-Path $PSScriptRoot 'conversation-memory-profile.ps1'
$passReportPath = Join-Path $env:TEMP "agent-builder-memory-pass-$([guid]::NewGuid().ToString('N')).json"
$failureReportPath = Join-Path $env:TEMP "agent-builder-memory-failure-$([guid]::NewGuid().ToString('N')).json"

try {
  & $profileScript `
    -ProcessName powershell `
    -RootProcessId $PID `
    -Scenario custom `
    -DurationSeconds 1 `
    -IntervalSeconds 0.25 `
    -MinSampleCount 2 `
    -MaxPrivateMB 10000 `
    -MaxWorkingSetMB 10000 `
    -MaxGrowthMBPerMinute 10000 `
    -MaxRendererPrivateMB 10000 `
    -RendererSustainedSamples 2 `
    -MaxRecoveryDeltaMB 10000 `
    -OutputPath $passReportPath | Out-Null

  $passReport = Get-Content -LiteralPath $passReportPath -Raw | ConvertFrom-Json
  if (-not $passReport.passed) { throw 'expected the permissive profile to pass' }
  if ($passReport.sampleCount -lt 2) { throw 'expected multiple samples for slope analysis' }
  if ($passReport.analysisSampleCount -lt 2) { throw 'expected multiple post-warmup samples' }
  if ($passReport.gates.Count -ne 7) { throw "expected 7 structured gates, got $($passReport.gates.Count)" }
  if ($null -eq $passReport.growthMBPerMinute) { throw 'missing growthMBPerMinute' }
  if ($null -eq $passReport.recoveryDeltaMB) { throw 'missing recoveryDeltaMB' }

  $threw = $false
  try {
    & $profileScript `
      -ProcessName powershell `
      -RootProcessId $PID `
      -Scenario fresh-quiet `
      -DurationSeconds 0 `
      -MinDurationSeconds 0 `
      -MaxPrivateMB 1 `
      -OutputPath $failureReportPath | Out-Null
  } catch {
    $threw = $true
  }
  $failureReport = Get-Content -LiteralPath $failureReportPath -Raw | ConvertFrom-Json
  $privateGate = $failureReport.gates | Where-Object name -eq 'peak-private'
  if (-not $threw) { throw 'expected a failed gate to produce a non-zero script result' }
  if ($failureReport.passed) { throw 'expected the failure report to be marked failed' }
  if ($privateGate.passed) { throw 'expected the peak-private gate to fail' }
  if ($failureReport.scenario -ne 'fresh-quiet') { throw 'scenario label was not preserved' }

  Write-Host 'Conversation memory profile contract smoke passed.'
} finally {
  Remove-Item -LiteralPath $passReportPath -Force -ErrorAction SilentlyContinue
  Remove-Item -LiteralPath $failureReportPath -Force -ErrorAction SilentlyContinue
}
