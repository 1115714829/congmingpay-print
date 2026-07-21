# Embed comctl32 v6 manifest for Walk (TTM_ADDTOOL). Needed for both 386 and amd64 debug.
$ErrorActionPreference = 'Stop'
Set-Location -Path (Split-Path -Parent $PSScriptRoot)

$rsrc = Join-Path $env:USERPROFILE 'go\bin\rsrc.exe'
if (-not (Test-Path $rsrc)) {
    Write-Host 'Installing rsrc ...'
    go install github.com/akavel/rsrc@latest
    if ($LASTEXITCODE -ne 0) { throw "go install rsrc failed (exit=$LASTEXITCODE)" }
}

if (-not (Test-Path '.\app.manifest')) {
    throw 'app.manifest not found in project root'
}

& $rsrc -arch 386 -manifest app.manifest -o rsrc_windows_386.syso
if ($LASTEXITCODE -ne 0 -or -not (Test-Path '.\rsrc_windows_386.syso')) {
    throw "rsrc 386 failed (exit=$LASTEXITCODE)"
}
& $rsrc -arch amd64 -manifest app.manifest -o rsrc_windows_amd64.syso
if ($LASTEXITCODE -ne 0 -or -not (Test-Path '.\rsrc_windows_amd64.syso')) {
    throw "rsrc amd64 failed (exit=$LASTEXITCODE)"
}

Write-Host 'embed-manifest OK: rsrc_windows_386.syso + rsrc_windows_amd64.syso'
