# 在仓库外的临时目录生成 GORM benchmark，再执行并输出结果。
# 目的：项目本体不引入 gorm 依赖，比较时由脚本临时拉取。
#
# 用法:
#   .\scripts\bench_gorm_temp_60s.ps1 -Scenario findone -BenchSeconds 60
#   .\scripts\bench_gorm_temp_60s.ps1 -Scenario list100 -BenchSeconds 60
#
param(
    [ValidateSet("findone", "list100", "list500", "list1000", "list2000")]
    [string]$Scenario,
    [int]$BenchSeconds = 60,
    [string]$RepoRoot = (Split-Path -Parent $PSScriptRoot)
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path (Join-Path $RepoRoot "resource\\mysql.json"))) {
    throw "resource/mysql.json not found under $RepoRoot"
}

$cfg = Get-Content (Join-Path $RepoRoot "resource\\mysql.json") -Raw | ConvertFrom-Json

function Get-OrDefault($v, $d) {
    if ($null -eq $v -or $v -eq "" -or $v -eq 0) { return $d }
    return $v
}

$charset = Get-OrDefault $cfg.Charset "utf8mb4"
$location = Get-OrDefault $cfg.Location "UTC"
$timeoutSec = 10
if ($cfg.Timeout -gt 0) {
    $timeoutSec = [int]($cfg.Timeout / 1000)
    if ($timeoutSec -le 0) { $timeoutSec = 10 }
}
$maxOpen = Get-OrDefault $cfg.MaxOpenConns 100
$maxIdle = Get-OrDefault $cfg.MaxIdleConns 10
$connLife = Get-OrDefault $cfg.ConnMaxLifetime 3600
$connIdle = Get-OrDefault $cfg.ConnMaxIdleTime 300

$u = [System.Uri]::EscapeDataString([string]$cfg.Username)
$p = [System.Uri]::EscapeDataString([string]$cfg.Password)
$locEsc = [System.Uri]::EscapeDataString([string]$location)
$dsn = "$u`:$p@tcp($($cfg.Host):$($cfg.Port))/$($cfg.Database)?charset=$charset&loc=$locEsc&timeout=${timeoutSec}s&readTimeout=${timeoutSec}s&writeTimeout=${timeoutSec}s"

$benchPattern = switch ($Scenario) {
    "findone" { "^BenchmarkGormFindOne$" }
    "list100" { "BenchmarkGormFindList/100_records" }
    "list500" { "BenchmarkGormFindList/500_records" }
    "list1000" { "BenchmarkGormFindList/1000_records" }
    "list2000" { "BenchmarkGormFindList/2000_records" }
}

$tmpRoot = Join-Path $env:TEMP "freego-gorm-bench-temp"
New-Item -ItemType Directory -Force -Path $tmpRoot | Out-Null

$goMod = @"
module gormbenchtmp

go 1.26

require (
    gorm.io/driver/mysql v1.5.7
    gorm.io/gorm v1.25.12
)
"@

$benchTemplate = @'
package main

import (
    "testing"
    "time"
    "gorm.io/driver/mysql"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

const (
    dsn = "__DSN__"
    findOneID int64 = 1988433892066983936
    listMin int64 = 1988433892066983936
    listMax int64 = 1990301977933774874
    maxOpen = __MAX_OPEN__
    maxIdle = __MAX_IDLE__
    connLifeSec = __CONN_LIFE__
    connIdleSec = __CONN_IDLE__
)

type ow struct {
    Id           int64  `gorm:"column:id;primaryKey"`
    AppID        string `gorm:"column:appID"`
    WalletID     string `gorm:"column:walletID"`
    Alias        string `gorm:"column:alias"`
    IsTrust      int64  `gorm:"column:isTrust"`
    PasswordType int64  `gorm:"column:passwordType"`
    Password     []byte `gorm:"column:password"`
    AuthKey      string `gorm:"column:authKey"`
    RootPath     string `gorm:"column:rootPath"`
    AccountIndex int64  `gorm:"column:accountIndex"`
    Keystore     string `gorm:"column:keyJson"`
    Applytime    int64  `gorm:"column:applytime"`
    Succtime     int64  `gorm:"column:succtime"`
    Dealstate    int64  `gorm:"column:dealstate"`
    Ctime        int64  `gorm:"column:ctime"`
    Utime        int64  `gorm:"column:utime"`
    State        int64  `gorm:"column:state"`
}

func (ow) TableName() string { return "ow_wallet" }

func open(b *testing.B) *gorm.DB {
    db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Silent),
        SkipDefaultTransaction: true,
        PrepareStmt: true,
    })
    if err != nil { b.Fatal(err) }
    s, err := db.DB()
    if err != nil { b.Fatal(err) }
    s.SetMaxOpenConns(maxOpen)
    s.SetMaxIdleConns(maxIdle)
    s.SetConnMaxLifetime(time.Duration(connLifeSec) * time.Second)
    s.SetConnMaxIdleTime(time.Duration(connIdleSec) * time.Second)
    return db
}

func BenchmarkGormFindOne(b *testing.B) {
    db := open(b)
    s, _ := db.DB()
    defer s.Close()
    var warm ow
    if r := db.Where("id = ?", findOneID).First(&warm); r.Error != nil { b.Fatal(r.Error) }
    b.ReportAllocs()
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            var row ow
            if r := db.Where("id = ?", findOneID).First(&row); r.Error != nil {
                b.Error(r.Error)
            }
        }
    })
}

func BenchmarkGormFindList(b *testing.B) {
    db := open(b)
    s, _ := db.DB()
    defer s.Close()
    cases := []struct{
        name string
        size int
    }{
        {"100_records", 100},
        {"500_records", 500},
        {"1000_records", 1000},
        {"2000_records", 2000},
    }
    for _, c := range cases {
        b.Run(c.name, func(b *testing.B) {
            warm := make([]ow, 0, c.size)
            if r := db.Where("id BETWEEN ? AND ?", listMin, listMax).Order("id DESC").Limit(c.size).Find(&warm); r.Error != nil {
                b.Fatal(r.Error)
            }
            b.ReportAllocs()
            b.ResetTimer()
            b.RunParallel(func(pb *testing.PB) {
                for pb.Next() {
                    rows := make([]ow, 0, c.size)
                    if r := db.Where("id BETWEEN ? AND ?", listMin, listMax).Order("id DESC").Limit(c.size).Find(&rows); r.Error != nil {
                        b.Error(r.Error)
                    }
                }
            })
        })
    }
}
'@

$benchFile = $benchTemplate.Replace("__DSN__", $dsn).Replace("__MAX_OPEN__", [string]$maxOpen).Replace("__MAX_IDLE__", [string]$maxIdle).Replace("__CONN_LIFE__", [string]$connLife).Replace("__CONN_IDLE__", [string]$connIdle)

$enc = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText((Join-Path $tmpRoot "go.mod"), $goMod, $enc)
[System.IO.File]::WriteAllText((Join-Path $tmpRoot "bench_test.go"), $benchFile, $enc)

Push-Location $tmpRoot
try {
    & go test -mod=mod '-run=^$' "-bench=$benchPattern" -benchmem "-benchtime=${BenchSeconds}s" -count=1 .
}
finally {
    Pop-Location
}
