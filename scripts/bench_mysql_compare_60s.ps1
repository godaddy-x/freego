# 独立压测（默认每项 60 秒）：FreeGo 基准直接在仓库跑；
# GORM 基准通过脚本在临时目录动态生成，不在仓库内引入 gorm 依赖。
#
# 用法:
#   .\scripts\bench_mysql_compare_60s.ps1
#   .\scripts\bench_mysql_compare_60s.ps1 -BenchSeconds 120
param(
    [int]$BenchSeconds = 60
)

$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

$benchTime = "${BenchSeconds}s"
$logFile = Join-Path $repoRoot "bench_60s_isolated.log"
$stamp = Get-Date -Format "yyyy-MM-dd HH:mm:ss zzz"

function Write-Log([string]$msg) {
    Write-Host $msg
    Add-Content -Path $logFile -Value $msg -Encoding utf8
}

function Run-FreeGo([string]$title, [string]$pattern) {
    Write-Log ""
    Write-Log "--- $title | FreeGo | -bench=$pattern | -benchtime=$benchTime ---"
    $out = & go test '-run=^$' "-bench=$pattern" -benchmem "-benchtime=$benchTime" -count=1 . 2>&1
    Write-Log ($out | Out-String)
}

function Run-GormTemp([string]$title, [string]$scenario) {
    Write-Log ""
    Write-Log "--- $title | GORM(temp script) | scenario=$scenario | -benchtime=$benchTime ---"
    $out = & (Join-Path $PSScriptRoot "bench_gorm_temp_60s.ps1") -Scenario $scenario -BenchSeconds $BenchSeconds -RepoRoot $repoRoot 2>&1
    Write-Log ($out | Out-String)
}

Write-Log ""
Write-Log "========== $stamp benchtime=$benchTime isolated =========="

Run-FreeGo  "FindOne"      "^BenchmarkMysqlFindOne$"
Run-GormTemp "FindOne"     "findone"

Run-FreeGo  "FindList 100" "BenchmarkMysqlFindList/100_records"
Run-GormTemp "FindList 100" "list100"

Run-FreeGo  "FindList 500" "BenchmarkMysqlFindList/500_records"
Run-GormTemp "FindList 500" "list500"

Run-FreeGo  "FindList 1000" "BenchmarkMysqlFindList/1000_records"
Run-GormTemp "FindList 1000" "list1000"

Run-FreeGo  "FindList 2000" "BenchmarkMysqlFindList/2000_records"
Run-GormTemp "FindList 2000" "list2000"

Write-Log ""
Write-Log "========== done $stamp =========="
