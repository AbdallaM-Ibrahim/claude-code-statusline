<#
.SYNOPSIS
    End-to-end wall clock for one status line render, process creation included.

.DESCRIPTION
    The Go benchmarks in bench_test.go measure the render itself. They cannot
    measure what dominated the version this replaces: creating four processes per
    tick. This script measures the whole thing the way Claude Code invokes it --
    spawn, write the payload on stdin, read two lines, exit.

    Both arms are interleaved iteration by iteration, so a load spike hits them
    equally. Absolute numbers move with the machine; the ratio between the arms is
    the durable result.

    A `bun -e ''` calibration row is included when bun is present: bare
    interpreter startup is the single number that makes results comparable across
    machines, because it is what inflates under load.

.PARAMETER Iterations
    Iterations per arm. 30 is enough for a stable median on a 4-core box.

.PARAMETER Load
    Saturate every logical core with busy work for the duration of the run. This
    is the condition that matters: a status line is slowest exactly when the
    machine is busy, which is when you are most likely to be looking at it.

.PARAMETER Isolate
    Point CLAUDE_CONFIG_DIR at a scratch directory. Keeps the run from touching
    real cache files, at the cost of measuring an empty transcript history -- so
    the money segments do less work than they would in life.

.PARAMETER Go
    The compiled binary. Defaults to the one this repository installs.

.PARAMETER Js
    The old bun/TypeScript entry point to compare against. Skipped when absent,
    which is the normal case for anyone who never ran that version.

.PARAMETER Repo
    Repository line 1 should describe. Defaults to this checkout.

.PARAMETER Transcript
    A transcript to name in the payload, defaulting to the most recently
    modified one. This matters for the comparison: the old version derives its
    money segments from transcript_path by shelling out to ccusage, so with the
    field empty it skips that work entirely and the arms are not doing the same
    job. The Go binary ignores the field -- it scans the projects directory --
    so supplying it changes only the arm being compared against.

.EXAMPLE
    pwsh bench/e2e.ps1
    pwsh bench/e2e.ps1 -Load -Iterations 30
    pwsh bench/e2e.ps1 -OutFile bench/results.json
#>

[CmdletBinding()]
param(
    [int]$Iterations = 30,
    [switch]$Load,
    [switch]$Isolate,
    [string]$Go = "",
    [string]$Js = "",
    [string]$Repo = "",
    [string]$Transcript = "",
    [string]$OutFile = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
if (-not $Repo) { $Repo = $repoRoot }

if (-not $Go) {
    # $IsWindows is PowerShell 7+; 5.1 falls back to the environment.
    $onWindows = ($PSVersionTable.PSVersion.Major -lt 6) -or $IsWindows
    $exe = if ($onWindows) { "statusline.exe" } else { "statusline" }
    $Go = Join-Path $HOME ".claude/$exe"
}
if (-not (Test-Path $Go)) {
    throw "binary not found at $Go -- build it first (go build -o `"$Go`" ./cmd/statusline) or pass -Go"
}
if (-not $Js) {
    $candidate = Join-Path $HOME ".claude/statusline.js"
    if (Test-Path $candidate) { $Js = $candidate }
}

# A payload with every optional field populated, so no segment is skipped for
# want of data. This is the shape Claude Code actually sends.
$payload = [ordered]@{
    session_id      = "741a0639-b2eb-42f0-8f7e-5827c5932a1f"
    transcript_path = $Transcript
    cwd             = $Repo
    model           = [ordered]@{ id = "claude-opus-5"; display_name = "Opus 5" }
    workspace       = [ordered]@{ current_dir = $Repo; project_dir = $Repo }
    cost            = [ordered]@{ total_cost_usd = 1.42 }
    context_window  = [ordered]@{
        context_window_size = 200000
        total_input_tokens  = 68000
        used_percentage     = 34
    }
    effort          = [ordered]@{ level = "xhigh" }
    thinking        = [ordered]@{ enabled = $true }
    fast_mode       = $false
    rate_limits     = [ordered]@{
        five_hour = [ordered]@{
            used_percentage = 42
            resets_at       = [int]((Get-Date).ToUniversalTime() - [datetime]"1970-01-01").TotalSeconds + 7200
        }
        seven_day = [ordered]@{ used_percentage = 18 }
    }
} | ConvertTo-Json -Depth 6 -Compress

function Format-Arg {
    param([string]$Value)
    if ($Value -eq "") { return '""' }
    if ($Value -match '[\s"]') { return '"' + ($Value -replace '"', '\"') + '"' }
    return $Value
}

function Measure-Once {
    param([string]$File, [string[]]$Arguments, [string]$Stdin)

    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $File
    # ProcessStartInfo.ArgumentList does not exist on .NET Framework, which is what
    # Windows PowerShell 5.1 runs on -- so the arguments are quoted by hand.
    $psi.Arguments = (($Arguments | ForEach-Object { Format-Arg $_ }) -join " ")
    $psi.RedirectStandardInput = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.UseShellExecute = $false

    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $p = [System.Diagnostics.Process]::Start($psi)
    $p.StandardInput.Write($Stdin)
    $p.StandardInput.Close()
    $out = $p.StandardOutput.ReadToEnd()
    $null = $p.StandardError.ReadToEnd()
    $p.WaitForExit()
    $sw.Stop()

    [pscustomobject]@{
        Ms       = $sw.Elapsed.TotalMilliseconds
        ExitCode = $p.ExitCode
        Lines    = ($out -split "`n" | Where-Object { $_ -ne "" }).Count
    }
}

function Get-Percentile {
    param([double[]]$Values, [double]$P)
    $sorted = $Values | Sort-Object
    if ($sorted.Count -eq 1) { return $sorted[0] }
    $idx = [math]::Min($sorted.Count - 1, [math]::Max(0, [int][math]::Round(($sorted.Count - 1) * $P)))
    $sorted[$idx]
}

function Summarise {
    param([string]$Name, [double[]]$Values)
    [pscustomobject]@{
        name   = $Name
        n      = $Values.Count
        min    = [math]::Round(($Values | Measure-Object -Minimum).Minimum, 1)
        median = [math]::Round((Get-Percentile -Values $Values -P 0.5), 1)
        p90    = [math]::Round((Get-Percentile -Values $Values -P 0.9), 1)
        max    = [math]::Round(($Values | Measure-Object -Maximum).Maximum, 1)
    }
}

# Resolved before any -Isolate redirection, so the transcript is a real one.
if (-not $Transcript) {
    $configDir = if ($env:CLAUDE_CONFIG_DIR) { $env:CLAUDE_CONFIG_DIR } else { Join-Path $HOME ".claude" }
    $projects = Join-Path $configDir "projects"
    if (Test-Path $projects) {
        $newest = Get-ChildItem -Path $projects -Filter *.jsonl -Recurse -ErrorAction SilentlyContinue |
            Sort-Object LastWriteTime -Descending | Select-Object -First 1
        if ($newest) { $Transcript = $newest.FullName }
    }
}

$bun = Get-Command bun -ErrorAction SilentlyContinue
$cores = [Environment]::ProcessorCount

$scratch = $null
$savedConfigDir = $env:CLAUDE_CONFIG_DIR
if ($Isolate) {
    $scratch = Join-Path ([System.IO.Path]::GetTempPath()) ("statusline-bench-" + [guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Force $scratch | Out-Null
    $env:CLAUDE_CONFIG_DIR = $scratch
}

Write-Host ""
Write-Host "machine   : $cores logical cores, PowerShell $($PSVersionTable.PSVersion)"
Write-Host "binary    : $Go"
Write-Host "old (bun) : $(if ($Js) { $Js } else { '(absent -- Go arm only)' })"
Write-Host "repo      : $Repo"
Write-Host "transcript: $(if ($Transcript) { $Transcript } else { '(none -- the bun arm will skip its cost work)' })"
Write-Host "config    : $(if ($Isolate) { "isolated at $scratch" } else { 'the real ~/.claude' })"
Write-Host "load      : $(if ($Load) { "saturating $cores cores" } else { 'idle' })"
Write-Host "iterations: $Iterations per arm, interleaved"
Write-Host ""

$loadJobs = @()
try {
    if ($Load) {
        # Busy loops, one per core. Started before the first measurement so both
        # arms see the same contention.
        $loadJobs = 1..$cores | ForEach-Object {
            Start-Job -ScriptBlock {
                $deadline = (Get-Date).AddMinutes(20)
                while ((Get-Date) -lt $deadline) { $null = 1 }
            }
        }
        Start-Sleep -Seconds 2 # let the load actually ramp
    }

    $goMs = [System.Collections.Generic.List[double]]::new()
    $jsMs = [System.Collections.Generic.List[double]]::new()
    $calMs = [System.Collections.Generic.List[double]]::new()

    # One untimed pass per arm: the first run pays for the OS file cache and, for
    # the Go binary, for building the cost state from scratch.
    $warm = Measure-Once -File $Go -Arguments @() -Stdin $payload
    if ($warm.ExitCode -ne 0) { throw "$Go exited $($warm.ExitCode)" }
    if ($warm.Lines -ne 2) { Write-Warning "expected 2 rendered lines, got $($warm.Lines)" }
    if ($Js -and $bun) { $null = Measure-Once -File $bun.Source -Arguments @($Js) -Stdin $payload }

    for ($i = 0; $i -lt $Iterations; $i++) {
        $goMs.Add((Measure-Once -File $Go -Arguments @() -Stdin $payload).Ms)

        if ($Js -and $bun) {
            $jsMs.Add((Measure-Once -File $bun.Source -Arguments @($Js) -Stdin $payload).Ms)
        }
        if ($bun) {
            $calMs.Add((Measure-Once -File $bun.Source -Arguments @("-e", "") -Stdin "").Ms)
        }

        if (($i + 1) % 10 -eq 0) { Write-Host "  $($i + 1)/$Iterations" }
    }
}
finally {
    if ($loadJobs) { $loadJobs | Stop-Job -PassThru | Remove-Job -Force }
    if ($Isolate) {
        if ($null -eq $savedConfigDir) {
            Remove-Item Env:CLAUDE_CONFIG_DIR -ErrorAction SilentlyContinue
        } else {
            $env:CLAUDE_CONFIG_DIR = $savedConfigDir
        }
    }
}

$rows = @(Summarise -Name "statusline (Go binary)" -Values $goMs.ToArray())
if ($jsMs.Count) { $rows += Summarise -Name "statusline.js (bun + subprocesses)" -Values $jsMs.ToArray() }
if ($calMs.Count) { $rows += Summarise -Name "bun -e '' (calibration)" -Values $calMs.ToArray() }

Write-Host ""
Write-Host "| arm | n | min | median | p90 | max |"
Write-Host "|---|---|---|---|---|---|"
foreach ($r in $rows) {
    Write-Host ("| {0} | {1} | {2} ms | {3} ms | {4} ms | {5} ms |" -f $r.name, $r.n, $r.min, $r.median, $r.p90, $r.max)
}

if ($jsMs.Count) {
    $ratio = [math]::Round((Summarise -Name x -Values $jsMs.ToArray()).median / (Summarise -Name x -Values $goMs.ToArray()).median, 1)
    Write-Host ""
    Write-Host "median speedup vs the bun version: ${ratio}x"
}

if ($OutFile) {
    $record = [ordered]@{
        cores      = $cores
        load       = [bool]$Load
        isolated   = [bool]$Isolate
        iterations = $Iterations
        binary     = $Go
        js         = $Js
        transcript = $Transcript
        rows       = $rows
    }
    $record | ConvertTo-Json -Depth 6 | Out-File -FilePath $OutFile -Encoding utf8
    Write-Host ""
    Write-Host "written: $OutFile"
}
