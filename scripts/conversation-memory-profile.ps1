param(
  [string]$ProcessName = 'AgentBuilder',
  [double]$MaxPrivateMB = 0,
  [string]$OutputPath = ''
)

$roots = @(Get-CimInstance Win32_Process | Where-Object { $_.Name -eq "$ProcessName.exe" -or $_.Name -eq $ProcessName })
if ($roots.Count -eq 0) { throw "Process $ProcessName was not found" }
$all = @(Get-CimInstance Win32_Process)
$ids = [System.Collections.Generic.HashSet[uint32]]::new()
$queue = [System.Collections.Generic.Queue[uint32]]::new()
foreach ($root in $roots) { [void]$ids.Add([uint32]$root.ProcessId); $queue.Enqueue([uint32]$root.ProcessId) }
while ($queue.Count -gt 0) {
  $parent = $queue.Dequeue()
  foreach ($child in $all | Where-Object ParentProcessId -eq $parent) {
    if ($ids.Add([uint32]$child.ProcessId)) { $queue.Enqueue([uint32]$child.ProcessId) }
  }
}
$rows = foreach ($id in $ids) {
  $process = Get-Process -Id $id -ErrorAction SilentlyContinue
  $cim = $all | Where-Object ProcessId -eq $id | Select-Object -First 1
  if ($process) {
    [pscustomobject]@{
      name = $process.ProcessName
      id = $id
      parentId = $cim.ParentProcessId
      privateMB = [math]::Round($process.PrivateMemorySize64 / 1MB, 2)
      workingSetMB = [math]::Round($process.WorkingSet64 / 1MB, 2)
      commandLine = $cim.CommandLine
    }
  }
}
$report = [pscustomobject]@{
  capturedAt = (Get-Date).ToUniversalTime().ToString('o')
  rootProcess = $ProcessName
  totalPrivateMB = [math]::Round(($rows | Measure-Object privateMB -Sum).Sum, 2)
  totalWorkingSetMB = [math]::Round(($rows | Measure-Object workingSetMB -Sum).Sum, 2)
  processes = @($rows | Sort-Object privateMB -Descending)
}
$json = $report | ConvertTo-Json -Depth 5
if ($OutputPath) { Set-Content -LiteralPath $OutputPath -Value $json -Encoding utf8 }
$json
if ($MaxPrivateMB -gt 0 -and $report.totalPrivateMB -gt $MaxPrivateMB) { throw "Private memory $($report.totalPrivateMB) MB exceeds $MaxPrivateMB MB" }
