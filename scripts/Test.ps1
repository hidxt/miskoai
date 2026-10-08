# Native development verification with scratch paths inside the allowed checkout.
$ErrorActionPreference = 'Stop'
$testRoot = Split-Path -Parent $PSScriptRoot
$env:GOPATH = Join-Path $testRoot '.tools\go'
$env:GOMODCACHE = Join-Path $testRoot '.tools\gomod'
$env:GOCACHE = Join-Path $testRoot '.tools\gocache'
$env:GOTMPDIR = Join-Path $testRoot '.tools\tmp'
$env:TEMP = $env:GOTMPDIR
$env:TMP = $env:GOTMPDIR
New-Item -ItemType Directory -Force -Path $env:GOTMPDIR | Out-Null
Push-Location $testRoot
try {
    go test ./... -count=1
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    go vet ./...
    exit $LASTEXITCODE
} finally { Pop-Location }
