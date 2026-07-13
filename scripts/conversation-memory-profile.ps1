param(
  [string]$ProcessName = 'AgentBuilder',
  [int]$RootProcessId = 0,
  [double]$MaxPrivateMB = 0,
  [int]$DurationSeconds = 0,
  [double]$IntervalSeconds = 2,
  [string]$OutputPath = ''
)

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
    byCategory = $byCategory
    processes = @($rows | Sort-Object privateMB -Descending)
  }
}

$duration = [math]::Max(0, $DurationSeconds)
$interval = [math]::Max(0.25, $IntervalSeconds)
$startedAt = Get-Date
$samples = [System.Collections.Generic.List[object]]::new()
Write-Host "Monitoring $ProcessName memory for $duration second(s); sampling every $interval second(s)..."
do {
  $sample = Get-AgentBuilderMemorySample
  $samples.Add($sample)
  $elapsed = [math]::Round(((Get-Date) - $startedAt).TotalSeconds, 1)
  $currentPeak = ($samples | Measure-Object totalPrivateMB -Maximum).Maximum
  Write-Host ("[{0,6}s] private={1,8} MB  working-set={2,8} MB  peak-private={3,8} MB" -f $elapsed, $sample.totalPrivateMB, $sample.totalWorkingSetMB, $currentPeak)
  if ($duration -le 0 -or ((Get-Date) - $startedAt).TotalSeconds -ge $duration) { break }
  Start-Sleep -Milliseconds ([int]($interval * 1000))
} while ($true)

$latest = $samples[$samples.Count - 1]
$peakPrivate = ($samples | Measure-Object totalPrivateMB -Maximum).Maximum
$peakWorkingSet = ($samples | Measure-Object totalWorkingSetMB -Maximum).Maximum
$report = [pscustomobject]@{
  capturedAt = $latest.capturedAt
  rootProcess = $ProcessName
  rootProcessId = $RootProcessId
  durationSeconds = [math]::Round(((Get-Date) - $startedAt).TotalSeconds, 2)
  sampleCount = $samples.Count
  totalPrivateMB = $latest.totalPrivateMB
  totalWorkingSetMB = $latest.totalWorkingSetMB
  peakTotalPrivateMB = [math]::Round($peakPrivate, 2)
  peakTotalWorkingSetMB = [math]::Round($peakWorkingSet, 2)
  byCategory = $latest.byCategory
  processes = $latest.processes
  samples = @($samples | ForEach-Object {
    [pscustomobject]@{
      capturedAt = $_.capturedAt
      totalPrivateMB = $_.totalPrivateMB
      totalWorkingSetMB = $_.totalWorkingSetMB
      byCategory = $_.byCategory
    }
  })
}
$json = $report | ConvertTo-Json -Depth 7
if ($OutputPath) { Set-Content -LiteralPath $OutputPath -Value $json -Encoding utf8 }
if ($OutputPath) { Write-Host "Memory profile written to $OutputPath" }
Write-Host "Completed: peak private $($report.peakTotalPrivateMB) MB; peak working set $($report.peakTotalWorkingSetMB) MB."
$json
if ($MaxPrivateMB -gt 0 -and $report.peakTotalPrivateMB -gt $MaxPrivateMB) {
  throw "Peak private memory $($report.peakTotalPrivateMB) MB exceeds $MaxPrivateMB MB"
}
