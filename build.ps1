# Cross-compiles every target into dist/ and installs the host binary.
#
#   .\build.ps1            build all targets + install to ~/.claude
#   .\build.ps1 -Install   install only, no cross-compile
#
# Pure Go, no cgo, so every target builds from Windows without a toolchain per
# platform.

param([switch]$Install)

$ErrorActionPreference = "Stop"

# Go may be on the machine PATH but absent from a shell whose environment was
# captured before it was installed.
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    $candidate = "C:\Program Files\Go\bin"
    if (Test-Path (Join-Path $candidate "go.exe")) {
        $env:Path = "$candidate;$env:Path"
    } else {
        throw "go not found on PATH and not at $candidate"
    }
}

$root = $PSScriptRoot
Set-Location $root

Write-Host "go $(go version)"

Write-Host "`n== tests ==" -ForegroundColor Cyan
go vet ./...
if ($LASTEXITCODE -ne 0) { throw "go vet failed" }
go test ./...
if ($LASTEXITCODE -ne 0) { throw "go test failed" }

$hostExe = Join-Path $HOME ".claude\statusline.exe"

if (-not $Install) {
    Write-Host "`n== cross-compile ==" -ForegroundColor Cyan
    $dist = Join-Path $root "dist"
    New-Item -ItemType Directory -Force $dist | Out-Null

    $targets = @(
        @{ os = "windows"; arch = "amd64"; ext = ".exe" },
        @{ os = "darwin";  arch = "arm64"; ext = "" },
        @{ os = "darwin";  arch = "amd64"; ext = "" },
        @{ os = "linux";   arch = "amd64"; ext = "" },
        @{ os = "linux";   arch = "arm64"; ext = "" }
    )

    foreach ($t in $targets) {
        $out = Join-Path $dist "statusline-$($t.os)-$($t.arch)$($t.ext)"
        $env:GOOS = $t.os
        $env:GOARCH = $t.arch
        $env:CGO_ENABLED = "0"
        go build -trimpath -ldflags "-s -w" -o $out ./cmd/statusline
        if ($LASTEXITCODE -ne 0) { throw "build failed for $($t.os)/$($t.arch)" }
        $mb = [math]::Round((Get-Item $out).Length / 1MB, 1)
        Write-Host ("  {0,-32} {1} MB" -f (Split-Path $out -Leaf), $mb)
    }

    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
}

Write-Host "`n== install ==" -ForegroundColor Cyan
go build -trimpath -ldflags "-s -w" -o $hostExe ./cmd/statusline
if ($LASTEXITCODE -ne 0) { throw "host build failed" }
Write-Host "  $hostExe  $([math]::Round((Get-Item $hostExe).Length / 1MB, 1)) MB"

Write-Host "`nDone. Point settings.json at the binary:" -ForegroundColor Green
Write-Host '  "command": "C:/Users/abdo/.claude/statusline.exe"'
