# congmingpay release build: 32-bit single exe compatible with Windows 7 -> 11.
#
# IMPORTANT: must build with the go1.20.14 toolchain (the last Go that supports Win7).
# Building with a newer Go (even if go.mod says "go 1.20") yields a binary that will
# NOT start on Windows 7. This script pins the go1.20.14 toolchain explicitly.
$ErrorActionPreference = 'Stop'
Set-Location -Path $PSScriptRoot

$go = Join-Path $env:USERPROFILE 'go\bin\go1.20.14.exe'
if (-not (Test-Path $go)) {
    # Fall back to the default 'go' only when it IS exactly go1.20.14
    # (machines where 1.20.14 is the default install). Keeps the Win7 pin intact.
    $ver = ''
    try { $ver = (& go version) } catch {}
    if ("$ver" -match 'go1\.20\.14') {
        $go = 'go'
    } else {
        Write-Error "go1.20.14 not found. Run: go install golang.org/dl/go1.20.14@latest ; go1.20.14 download"
    }
}

# Generate Windows resources: embed manifest (comctl32 v6 theme + asInvoker + Win7 compat).
# Without the comctl32 v6 manifest, Walk crashes at startup (TTM_ADDTOOL failed).
$rsrc = Join-Path $env:USERPROFILE 'go\bin\rsrc.exe'
if (-not (Test-Path $rsrc)) {
    Write-Host "Installing rsrc ..."
    & go install github.com/akavel/rsrc@latest
}
# Name it *_windows_386.syso so it is only linked for GOARCH=386 (keeps other arches clean).
& $rsrc -arch 386 -manifest app.manifest -o rsrc_windows_386.syso
if ($LASTEXITCODE -ne 0 -or -not (Test-Path rsrc_windows_386.syso)) {
    Write-Error "rsrc failed (exit=$LASTEXITCODE)."
}

# 32-bit, no console window, pure Go (no CGO).
$env:GOOS = 'windows'
$env:GOARCH = '386'
$env:CGO_ENABLED = '0'
& $go build -ldflags "-H windowsgui -s -w" -o congmingpay.exe .
if ($LASTEXITCODE -ne 0) { Write-Error "go build failed (exit=$LASTEXITCODE)." }

Write-Host "Build OK: congmingpay.exe (GOARCH=386, go1.20.14)"
