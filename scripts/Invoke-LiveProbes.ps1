# Opt-in Windows development probes. Costs and QR/account use require user authorization.
param([switch]$Weixin)
$ErrorActionPreference = 'Stop'
$probeRoot = Split-Path -Parent $PSScriptRoot
$probeBinary = Join-Path $probeRoot 'dist\miskoai-windows-amd64.exe'
if (-not (Test-Path -LiteralPath $probeBinary)) { throw 'Build the development Windows binary first.' }
if ($Weixin) {
    & $probeBinary weixin login
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Write-Output 'Send a synthetic direct text message from the QR-scanning user before the single-poll probe.'
    Read-Host 'Press Enter when the message has been sent' | Out-Null
    & $probeBinary poc weixin
    exit $LASTEXITCODE
}
$probeKeys = Join-Path $probeRoot 'secrets\api-keys.json'
$probeOwner = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
$probeAcl = Get-Acl -LiteralPath $probeKeys
if ($probeAcl.GetOwner([System.Security.Principal.SecurityIdentifier]) -ne $probeOwner -or -not $probeAcl.AreAccessRulesProtected) { throw 'Credential file requires current-owner and protected ACL.' }
foreach ($probeRule in $probeAcl.GetAccessRules($true, $true, [System.Security.Principal.SecurityIdentifier])) {
    if ($probeRule.AccessControlType -eq 'Allow' -and $probeRule.IdentityReference -ne $probeOwner) { throw 'Credential file allows another identity.' }
}
$probeConfig = [System.IO.File]::ReadAllText($probeKeys) | ConvertFrom-Json
$oldDeepSeek = $env:DEEPSEEK_API_KEY
$oldOllama = $env:OLLAMA_API_KEY
$oldOutputTokens = $env:MISKOAI_MAX_OUTPUT_TOKENS
try {
    $env:DEEPSEEK_API_KEY = $probeConfig.DEEPSEEK_API_KEY
    $env:OLLAMA_API_KEY = $probeConfig.OLLAMA_API_KEY
    $env:MISKOAI_MAX_OUTPUT_TOKENS = '64'
    foreach ($probeCommand in @('deepseek', 'stream', 'search')) {
        & $probeBinary poc $probeCommand
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }
    # Only a generated public synthetic fixture; never private user media.
    & $probeBinary poc vision (Join-Path $probeRoot 'internal\cli\testdata\synthetic.png')
    exit $LASTEXITCODE
} finally {
    $env:DEEPSEEK_API_KEY = $oldDeepSeek; $env:OLLAMA_API_KEY = $oldOllama
    $env:MISKOAI_MAX_OUTPUT_TOKENS = $oldOutputTokens
    $probeConfig = $null
}
