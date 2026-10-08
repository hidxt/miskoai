# Windows development helper only. Not part of the Linux deployment runtime.
# Prompts hide keys; the keys never occur in command arguments or shell history.
$ErrorActionPreference = 'Stop'
$credentialRoot = Split-Path -Parent $PSScriptRoot
$credentialDir = Join-Path $credentialRoot 'secrets'
$credentialPath = Join-Path $credentialDir 'api-keys.json'
if (Test-Path -LiteralPath $credentialPath) { throw 'Credential file already exists; inspect it before replacement.' }
New-Item -ItemType Directory -Path $credentialDir -Force | Out-Null
$credentialOwner = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
$credentialAcl = [System.Security.AccessControl.FileSecurity]::new()
$credentialAcl.SetOwner($credentialOwner)
$credentialAcl.SetAccessRuleProtection($true, $false)
$credentialAcl.AddAccessRule([System.Security.AccessControl.FileSystemAccessRule]::new($credentialOwner, 'FullControl', 'Allow'))
# Create with the restricted ACL atomically and hold an exclusive stream.
# Changing ACL after creation cannot revoke a reader that opened the empty file.
$credentialFile = [System.IO.FileSystemAclExtensions]::Create([System.IO.FileInfo]::new($credentialPath), [System.IO.FileMode]::CreateNew, [System.Security.AccessControl.FileSystemRights]::Write, [System.IO.FileShare]::None, 4096, [System.IO.FileOptions]::None, $credentialAcl)
function Read-TestKey([string]$Prompt) {
    $maskedTestKey = Read-Host $Prompt -AsSecureString
    $maskedPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($maskedTestKey)
    try { return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($maskedPointer) }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($maskedPointer); $maskedTestKey.Dispose() }
}
try {
    $testDeepSeek = Read-TestKey 'Temporary DeepSeek API Key'
    $testOllama = Read-TestKey 'Temporary Ollama API Key'
    $credentialJson = @{ DEEPSEEK_API_KEY = $testDeepSeek; OLLAMA_API_KEY = $testOllama } | ConvertTo-Json -Compress
    $credentialBytes = [System.Text.Encoding]::UTF8.GetBytes($credentialJson)
    $credentialFile.Write($credentialBytes, 0, $credentialBytes.Length)
    $credentialFile.Flush($true)
    Write-Output 'Temporary credentials saved in ignored, owner-restricted secrets/api-keys.json. Never paste it into chat.'
} finally {
    $credentialFile.Dispose()
    if ($null -ne $credentialBytes) {[System.Array]::Clear($credentialBytes, 0, $credentialBytes.Length)}
    $testDeepSeek = $null; $testOllama = $null; $credentialJson = $null
}
