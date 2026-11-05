[CmdletBinding()]
param(
    [string]$SecretDirectory
)

if ([string]::IsNullOrWhiteSpace($SecretDirectory)) {
    $SecretDirectory = Join-Path (Split-Path -Parent $PSScriptRoot) "secrets"
}
$secretFiles = @(
    "postgres_password.txt",
    "database_url.txt",
    "admin_password.txt",
    "encryption_key.txt"
)

foreach ($name in $secretFiles) {
    $path = Join-Path $SecretDirectory $name
    if (Test-Path -LiteralPath $path) {
        throw "Refusing to overwrite existing secret: $path"
    }
}

New-Item -ItemType Directory -Force -Path $SecretDirectory | Out-Null
$random = [Security.Cryptography.RandomNumberGenerator]::Create()
$utf8WithoutBom = New-Object Text.UTF8Encoding($false)

function New-RandomBytes([int]$count) {
    $bytes = New-Object byte[] $count
    $random.GetBytes($bytes)
    return ,$bytes
}

function ConvertTo-Hex([byte[]]$bytes) {
    return ([BitConverter]::ToString($bytes) -replace "-", "").ToLowerInvariant()
}

$databasePassword = ConvertTo-Hex (New-RandomBytes 24)
$adminPassword = ConvertTo-Hex (New-RandomBytes 24)
$encryptionKey = [Convert]::ToBase64String((New-RandomBytes 32))
$databaseURL = "postgres://gomaso:$databasePassword@postgres:5432/go_maso?sslmode=disable"

[IO.File]::WriteAllText((Join-Path $SecretDirectory "postgres_password.txt"), $databasePassword, $utf8WithoutBom)
[IO.File]::WriteAllText((Join-Path $SecretDirectory "database_url.txt"), $databaseURL, $utf8WithoutBom)
[IO.File]::WriteAllText((Join-Path $SecretDirectory "admin_password.txt"), $adminPassword, $utf8WithoutBom)
[IO.File]::WriteAllText((Join-Path $SecretDirectory "encryption_key.txt"), $encryptionKey, $utf8WithoutBom)

$random.Dispose()
Write-Host "Secrets created in $SecretDirectory. Retrieve the initial admin password from admin_password.txt and protect these files."
