param(
  [string]$ProcessName = 'AgentBuilder',
  [int]$RootProcessId = 0,
  [ValidateSet('custom', 'fresh-quiet', 'active-conversation', 'long-session', 'long-stream', 'recovery')]
  [string]$Scenario = 'custom',
  [double]$MaxPrivateMB = 0,
  [double]$MaxWorkingSetMB = 0,
  [double]$MaxGrowthMBPerMinute = 0,
  [double]$MaxRendererPrivateMB = 0,
  [int]$RendererSustainedSamples = 3,
  [double]$RecoveryBaselinePrivateMB = 0,
  [double]$MaxRecoveryDeltaMB = 0,
  [int]$WarmupSeconds = 0,
  [int]$MinDurationSeconds = 0,
  [int]$MinSampleCount = 1,
  [int]$DurationSeconds = 0,
  [double]$IntervalSeconds = 2,
  [string]$OutputPath = '',
  [switch]$NoJsonOutput
)

$requestedParameters = $PSBoundParameters

function Set-ScenarioDefaults {
  switch ($Scenario) {
    'fresh-quiet' {
      if (-not $requestedParameters.ContainsKey('DurationSeconds')) { $script:DurationSeconds = 600 }
      if (-not $requestedParameters.ContainsKey('MinDurationSeconds')) { $script:MinDurationSeconds = 600 }
      if (-not $requestedParameters.ContainsKey('MaxPrivateMB')) { $script:MaxPrivateMB = 700 }
    }
    'active-conversation' {
      if (-not $requestedParameters.ContainsKey('MaxPrivateMB')) { $script:MaxPrivateMB = 900 }
    }
    'long-session' {
      if (-not $requestedParameters.ContainsKey('MaxPrivateMB')) { $script:MaxPrivateMB = 900 }
    }
    'long-stream' {
      if (-not $requestedParameters.ContainsKey('DurationSeconds')) { $script:DurationSeconds = 3600 }
      if (-not $requestedParameters.ContainsKey('MinDurationSeconds')) { $script:MinDurationSeconds = 3600 }
      if (-not $requestedParameters.ContainsKey('WarmupSeconds')) { $script:WarmupSeconds = 300 }
      if (-not $requestedParameters.ContainsKey('MaxPrivateMB')) { $script:MaxPrivateMB = 1000 }
      if (-not $requestedParameters.ContainsKey('MaxGrowthMBPerMinute')) { $script:MaxGrowthMBPerMinute = 1 }
    }
    'recovery' {
      if (-not $requestedParameters.ContainsKey('DurationSeconds')) { $script:DurationSeconds = 60 }
      if (-not $requestedParameters.ContainsKey('MinDurationSeconds')) { $script:MinDurationSeconds = 60 }
      if (-not $requestedParameters.ContainsKey('MaxPrivateMB')) { $script:MaxPrivateMB = 1000 }
      if (-not $requestedParameters.ContainsKey('MaxRecoveryDeltaMB')) { $script:MaxRecoveryDeltaMB = 150 }
    }
  }
}

function Get-ProcessCategory($process, [System.Collections.Generic.HashSet[uint32]]$rootIds) {
  if ($rootIds.Contains([uint32]$process.ProcessId)) { return 'runtime' }
  if ($process.Name -ne 'msedgewebview2.exe') { return 'child' }
  $commandLine = [string]$process.CommandLine
  if ($commandLine -match '--type=renderer') { return 'webview-renderer' }
  if ($commandLine -match '--type=gpu-process') { return 'webview-gpu' }
  if ($commandLine -match '--type=utility') { return 'webview-utility' }
  if ($commandLine -match '--type=crashpad-handler') { return 'webview-crashpad' }
  return 'webview-browser'
}

function Get-AgentBuilderMemorySample {
  $all = @(Get-CimInstance Win32_Process)
  if ($RootProcessId -gt 0) {
    $roots = @($all | Where-Object ProcessId -eq $RootProcessId)
  } else {
    $roots = @($all | Where-Object { $_.Name -eq "$ProcessName.exe" -or $_.Name -eq $ProcessName })
  }
  if ($roots.Count -eq 0) { throw "Process $ProcessName was not found" }

  $rootIds = [System.Collections.Generic.HashSet[uint32]]::new()
  $ids = [System.Collections.Generic.HashSet[uint32]]::new()
  $queue = [System.Collections.Generic.Queue[uint32]]::new()
  foreach ($root in $roots) {
    [void]$rootIds.Add([uint32]$root.ProcessId)
    [void]$ids.Add([uint32]$root.ProcessId)
    $queue.Enqueue([uint32]$root.ProcessId)
  }
  while ($queue.Count -gt 0) {
    $parent = $queue.Dequeue()
    foreach ($child in $all | Where-Object ParentProcessId -eq $parent) {
      if ($ids.Add([uint32]$child.ProcessId)) { $queue.Enqueue([uint32]$child.ProcessId) }
    }
  }

  $rows = foreach ($processId in $ids) {
    $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
    $cim = $all | Where-Object ProcessId -eq $processId | Select-Object -First 1
    if ($process -and $cim) {
      [pscustomobject]@{
        name = $process.ProcessName
        category = Get-ProcessCategory $cim $rootIds
        id = $processId
        parentId = $cim.ParentProcessId
        privateMB = [math]::Round($process.PrivateMemorySize64 / 1MB, 2)
        workingSetMB = [math]::Round($process.WorkingSet64 / 1MB, 2)
        commandLine = $cim.CommandLine
      }
    }
  }
  $byCategory = @($rows | Group-Object category | ForEach-Object {
    [pscustomobject]@{
      category = $_.Name
      processCount = $_.Count
      privateMB = [math]::Round(($_.Group | Measure-Object privateMB -Sum).Sum, 2)
      workingSetMB = [math]::Round(($_.Group | Measure-Object workingSetMB -Sum).Sum, 2)
    }
  } | Sort-Object privateMB -Descending)
  return [pscustomobject]@{
    capturedAt = (Get-Date).ToUniversalTime().ToString('o')
    totalPrivateMB = [math]::Round(($rows | Measure-Object privateMB -Sum).Sum, 2)
    totalWorkingSetMB = [math]::Round(($rows | Measure-Object workingSetMB -Sum).Sum, 2)
    rendererPrivateMB = [math]::Round((@($byCategory | Where-Object category -eq 'webview-renderer') | Measure-Object privateMB -Sum).Sum, 2)
    byCategory = $byCategory
    processes = @($rows | Sort-Object privateMB -Descending)
  }
}

function Get-LinearGrowthMBPerMinute([object[]]$items) {
  if ($items.Count -lt 2) { return 0 }
  $meanX = ($items | Measure-Object elapsedSeconds -Average).Average
  $meanY = ($items | Measure-Object totalPrivateMB -Average).Average
  $numerator = 0.0
  $denominator = 0.0
  foreach ($item in $items) {
    $dx = [double]$item.elapsedSeconds - $meanX
    $numerator += $dx * ([double]$item.totalPrivateMB - $meanY)
    $denominator += $dx * $dx
  }
  if ($denominator -le 0) { return 0 }
  return [math]::Round(($numerator / $denominator) * 60, 4)
}

function Get-MaxConsecutiveRendererBreaches([object[]]$items, [double]$threshold) {
  if ($threshold -le 0) { return 0 }
  $current = 0
  $maximum = 0
  foreach ($item in $items) {
    if ([double]$item.rendererPrivateMB -gt $threshold) {
      $current++
      $maximum = [math]::Max($maximum, $current)
    } else {
      $current = 0
    }
  }
  return $maximum
}

function New-Gate([string]$name, [bool]$enabled, [double]$actual, [double]$limit, [string]$unit, [bool]$passed) {
  [pscustomobject]@{ name = $name; enabled = $enabled; passed = (-not $enabled) -or $passed; actual = $actual; limit = $limit; unit = $unit }
}

Set-ScenarioDefaults
$duration = [math]::Max(0, $DurationSeconds)
$interval = [math]::Max(0.25, $IntervalSeconds)
$startedAt = Get-Date
$samples = [System.Collections.Generic.List[object]]::new()
Write-Host "Monitoring $ProcessName memory for $duration second(s); scenario=$Scenario; sampling every $interval second(s)..."
do {
  $sample = Get-AgentBuilderMemorySample
  $elapsed = ((Get-Date) - $startedAt).TotalSeconds
  $sample | Add-Member -NotePropertyName elapsedSeconds -NotePropertyValue ([math]::Round($elapsed, 3))
  $samples.Add($sample)
  $currentPeak = ($samples | Measure-Object totalPrivateMB -Maximum).Maximum
  Write-Host ("[{0,6:N1}s] private={1,8} MB  working-set={2,8} MB  renderer={3,8} MB  peak-private={4,8} MB" -f $elapsed, $sample.totalPrivateMB, $sample.totalWorkingSetMB, $sample.rendererPrivateMB, $currentPeak)
  if ($duration -le 0 -or $elapsed -ge $duration) { break }
  Start-Sleep -Milliseconds ([int]($interval * 1000))
} while ($true)

$finishedAt = Get-Date
$actualDuration = ($finishedAt - $startedAt).TotalSeconds
$latest = $samples[$samples.Count - 1]
$peakPrivate = ($samples | Measure-Object totalPrivateMB -Maximum).Maximum
$peakWorkingSet = ($samples | Measure-Object totalWorkingSetMB -Maximum).Maximum
$analysisSamples = @($samples | Where-Object elapsedSeconds -ge $WarmupSeconds)
$growth = Get-LinearGrowthMBPerMinute $analysisSamples
$rendererBreaches = Get-MaxConsecutiveRendererBreaches $analysisSamples $MaxRendererPrivateMB
$baselinePrivate = if ($RecoveryBaselinePrivateMB -gt 0) { $RecoveryBaselinePrivateMB } else { [double]$samples[0].totalPrivateMB }
$recoveryDelta = [math]::Round([double]$latest.totalPrivateMB - $baselinePrivate, 2)
$gates = @(
  New-Gate 'minimum-duration' ($MinDurationSeconds -gt 0) ([math]::Round($actualDuration, 2)) $MinDurationSeconds 'seconds' ($actualDuration -ge $MinDurationSeconds)
  New-Gate 'minimum-samples' ($MinSampleCount -gt 1) $samples.Count $MinSampleCount 'samples' ($samples.Count -ge $MinSampleCount)
  New-Gate 'peak-private' ($MaxPrivateMB -gt 0) ([math]::Round($peakPrivate, 2)) $MaxPrivateMB 'MB' ($peakPrivate -le $MaxPrivateMB)
  New-Gate 'peak-working-set' ($MaxWorkingSetMB -gt 0) ([math]::Round($peakWorkingSet, 2)) $MaxWorkingSetMB 'MB' ($peakWorkingSet -le $MaxWorkingSetMB)
  New-Gate 'post-warmup-growth' ($MaxGrowthMBPerMinute -gt 0) $growth $MaxGrowthMBPerMinute 'MB/min' ($analysisSamples.Count -ge 2 -and $growth -le $MaxGrowthMBPerMinute)
  New-Gate 'renderer-sustained-private' ($MaxRendererPrivateMB -gt 0) $rendererBreaches $RendererSustainedSamples 'consecutive samples' ($rendererBreaches -lt $RendererSustainedSamples)
  New-Gate 'recovery-delta' ($MaxRecoveryDeltaMB -gt 0) $recoveryDelta $MaxRecoveryDeltaMB 'MB' ($recoveryDelta -le $MaxRecoveryDeltaMB)
)
$failedGates = @($gates | Where-Object { $_.enabled -and -not $_.passed })
$report = [pscustomobject]@{
  capturedAt = $latest.capturedAt
  scenario = $Scenario
  passed = $failedGates.Count -eq 0
  rootProcess = $ProcessName
  rootProcessId = $RootProcessId
  durationSeconds = [math]::Round($actualDuration, 2)
  warmupSeconds = $WarmupSeconds
  analysisSampleCount = $analysisSamples.Count
  sampleCount = $samples.Count
  totalPrivateMB = $latest.totalPrivateMB
  totalWorkingSetMB = $latest.totalWorkingSetMB
  rendererPrivateMB = $latest.rendererPrivateMB
  peakTotalPrivateMB = [math]::Round($peakPrivate, 2)
  peakTotalWorkingSetMB = [math]::Round($peakWorkingSet, 2)
  growthMBPerMinute = $growth
  recoveryBaselinePrivateMB = [math]::Round($baselinePrivate, 2)
  recoveryDeltaMB = $recoveryDelta
  gates = $gates
  byCategory = $latest.byCategory
  processes = $latest.processes
  samples = @($samples | ForEach-Object {
    [pscustomobject]@{
      capturedAt = $_.capturedAt
      elapsedSeconds = $_.elapsedSeconds
      totalPrivateMB = $_.totalPrivateMB
      totalWorkingSetMB = $_.totalWorkingSetMB
      rendererPrivateMB = $_.rendererPrivateMB
      byCategory = $_.byCategory
    }
  })
}
$json = $report | ConvertTo-Json -Depth 7
if ($OutputPath) { Set-Content -LiteralPath $OutputPath -Value $json -Encoding utf8 }
if ($OutputPath) { Write-Host "Memory profile written to $OutputPath" }
Write-Host "Completed: passed=$($report.passed); peak private $($report.peakTotalPrivateMB) MB; peak working set $($report.peakTotalWorkingSetMB) MB; post-warmup growth $($report.growthMBPerMinute) MB/min."
if (-not $NoJsonOutput) { $json }
if ($failedGates.Count -gt 0) {
  $summary = ($failedGates | ForEach-Object { "$($_.name): actual=$($_.actual) $($_.unit), limit=$($_.limit) $($_.unit)" }) -join '; '
  throw "Memory acceptance failed: $summary"
}
