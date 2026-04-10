param(
    [string]$Version = "",
    [string]$OutputDir = "dist",
    [string]$GoBinary = "",
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"

function Resolve-GoBinary {
    param([string]$Candidate)

    if ($Candidate) {
        if (-not (Test-Path $Candidate)) {
            throw "Specified Go binary does not exist: $Candidate"
        }
        return (Resolve-Path $Candidate).Path
    }

    $commands = @(
        (Get-Command go -ErrorAction SilentlyContinue),
        (Get-Item "C:\Program Files\Go\bin\go.exe" -ErrorAction SilentlyContinue)
    ) | Where-Object { $_ }

    if ($commands.Count -eq 0) {
        throw "Go was not found. Install Go or pass -GoBinary."
    }

    if ($commands[0] -is [System.Management.Automation.CommandInfo]) {
        return $commands[0].Source
    }

    return $commands[0].FullName
}

function New-ZipPackage {
    param(
        [string]$SourceFile,
        [string]$DestinationZip
    )

    if (Test-Path $DestinationZip) {
        Remove-Item $DestinationZip -Force
    }

    Compress-Archive -Path $SourceFile -DestinationPath $DestinationZip -CompressionLevel Optimal
}

if (-not $Version) {
    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $Version = "dev-$timestamp"
}

$go = Resolve-GoBinary -Candidate $GoBinary
$root = Split-Path -Parent $PSScriptRoot
$distRoot = Join-Path $root $OutputDir
$releaseDir = Join-Path $distRoot $Version

New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null

Push-Location $root
try {
    if (-not $SkipTests) {
        & $go test ./...
    }

    $targets = @(
        @{ GOOS = "windows"; GOARCH = "amd64"; Suffix = ".exe" },
        @{ GOOS = "linux"; GOARCH = "amd64"; Suffix = "" },
        @{ GOOS = "linux"; GOARCH = "arm64"; Suffix = "" },
        @{ GOOS = "darwin"; GOARCH = "amd64"; Suffix = "" },
        @{ GOOS = "darwin"; GOARCH = "arm64"; Suffix = "" }
    )

    $artifacts = @()
    foreach ($target in $targets) {
        $name = "injectctl-$Version-$($target.GOOS)-$($target.GOARCH)$($target.Suffix)"
        $binaryPath = Join-Path $releaseDir $name

        Write-Host "Building $name"
        $env:GOOS = $target.GOOS
        $env:GOARCH = $target.GOARCH
        $env:CGO_ENABLED = "0"

        & $go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $binaryPath ./cmd/injectctl

        $zipPath = "$binaryPath.zip"
        New-ZipPackage -SourceFile $binaryPath -DestinationZip $zipPath
        $artifacts += $binaryPath
        $artifacts += $zipPath
    }

    $checksumLines = foreach ($artifact in $artifacts) {
        $hash = Get-FileHash -Algorithm SHA256 -Path $artifact
        "{0}  {1}" -f $hash.Hash.ToLowerInvariant(), (Split-Path $artifact -Leaf)
    }
    $checksumPath = Join-Path $releaseDir "sha256sums.txt"
    Set-Content -Path $checksumPath -Value $checksumLines

    Write-Host ""
    Write-Host "Release artifacts written to $releaseDir"
    Write-Host "Checksums: $checksumPath"
}
finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Pop-Location
}
