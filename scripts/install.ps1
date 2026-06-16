# okf — One-click installer for Windows (PowerShell)
# Downloads pre-built binary from GitHub Releases
#
# Usage (PowerShell):
#   iwr -useb https://raw.githubusercontent.com/superops-team/okf/main/scripts/install.ps1 | iex
#   iwr -useb "https://raw.githubusercontent.com/superops-team/okf/main/scripts/install.ps1" -OutFile install.ps1; .\install.ps1 v1.2.0
#
# Environment variables:
#   $env:OKF_VERSION    - specific version to install (default: latest)
#   $env:OKF_INSTALL_DIR - install directory (default: $env:USERPROFILE\bin)

param(
    [Parameter(Position = 0)]
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
$GitHubRepo = "superops-team/okf"
$BinaryName = "okf"
$TmpDir     = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $TmpDir -Force | Out-Null

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
function Write-Step($msg)    { Write-Host "  `" -ForegroundColor Cyan -NoNewline; Write-Host $msg }
function Write-Ok($msg)      { Write-Host "  [OK] " -ForegroundColor Green -NoNewline; Write-Host $msg }
function Write-Warn($msg)    { Write-Host "  [WARN] " -ForegroundColor Yellow -NoNewline; Write-Host $msg }
function Write-Err($msg)     { Write-Host "  [ERR] " -ForegroundColor Red -NoNewline; Write-Host $msg }

function Write-Banner {
    Write-Host ""
    Write-Host "okf - Open Knowledge Format Installer" -ForegroundColor Cyan
    Write-Host "=======================================" -ForegroundColor Cyan
    Write-Host ""
}

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
Register-EngineEvent -SourceIdentifier PowerShell.Exiting -Action {
    if ($TmpDir -and (Test-Path $TmpDir)) { Remove-Item -Recurse -Force $TmpDir -ErrorAction SilentlyContinue }
} | Out-Null

# ---------------------------------------------------------------------------
# Detect OS / Arch
# ---------------------------------------------------------------------------
function Detect-Arch {
    $a = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    switch ($a) {
        { $_ -match "X64|Amd64" } { return "amd64" }
        { $_ -match "Arm64" }     { return "arm64" }
        default {
            Write-Err "Unsupported architecture: $a"
            exit 1
        }
    }
}

# ---------------------------------------------------------------------------
# Resolve latest version from GitHub API
# ---------------------------------------------------------------------------
function Resolve-LatestVersion {
    $url = "https://api.github.com/repos/$GitHubRepo/releases/latest"
    try {
        $headers = @{ "User-Agent" = "okf-installer" }
        $resp = Invoke-RestMethod -Uri $url -Headers $headers -UseBasicParsing -ErrorAction Stop
        return $resp.tag_name
    } catch {
        Write-Err "Failed to query GitHub API for latest release"
        Write-Warn "Specify a version explicitly: .\install.ps1 v1.2.0"
        exit 1
    }
}

# ---------------------------------------------------------------------------
# Downloads a file with retry
# ---------------------------------------------------------------------------
function Invoke-Download($Url, $Dest) {
    Write-Step "Downloading: $Url"
    $attempts = 0
    do {
        try {
            $attempts++
            Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing -TimeoutSec 60 -ErrorAction Stop
            return
        } catch {
            if ($attempts -ge 3) { throw }
            Write-Warn "Download attempt $attempts failed, retrying..."
            Start-Sleep -Seconds 2
        }
    } while ($true)
}

# ---------------------------------------------------------------------------
# Verify SHA256 checksum
# ---------------------------------------------------------------------------
function Verify-Checksum($AssetName, $ChecksumsFile) {
    if (-not (Test-Path $ChecksumsFile)) {
        Write-Warn "SHA256SUMS not found, skipping verification"
        return
    }
    $lines = Get-Content $ChecksumsFile
    $expected = $null
    foreach ($line in $lines) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -ge 2 -and $parts[1].Trim() -eq $AssetName) {
            $expected = $parts[0].Trim()
            break
        }
    }
    if (-not $expected) {
        Write-Warn "No checksum entry for $AssetName, skipping verification"
        return
    }
    $actual = (Get-FileHash -Algorithm SHA256 -Path $AssetName).Hash.ToLower()
    if ($actual -ne $expected.ToLower()) {
        Write-Err "Checksum mismatch for $AssetName"
        Write-Err "  expected: $expected"
        Write-Err "  actual  : $actual"
        exit 1
    }
    Write-Ok "Checksum verified (SHA256)"
}

# ---------------------------------------------------------------------------
# Expand zip (native .NET 4.5+/PowerShell 5.1+ / pwsh)
# ---------------------------------------------------------------------------
function Expand-Zip($Path, $Dest) {
    if ($PSVersionTable.PSEdition -eq "Core" -or $PSVersionTable.PSVersion.Major -ge 5) {
        Expand-Archive -Path $Path -DestinationPath $Dest -Force
        return
    }
    # fallback: Shell.Application
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    [System.IO.Compression.ZipFile]::ExtractToDirectory($Path, $Dest)
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
Write-Banner

$arch = Detect-Arch
Write-Step "Detected system: windows/$arch"

# Resolve version: param -> env -> latest
if ($Version) {
    $ver = $Version
} elseif ($env:OKF_VERSION) {
    $ver = $env:OKF_VERSION
} else {
    Write-Step "Resolving latest release version..."
    $ver = Resolve-LatestVersion
}
$ver = $ver.TrimStart('v')
Write-Step "Version: v$ver"

$assetName = "$BinaryName`_$ver`_windows_$arch.zip"
$downloadUrl = "https://github.com/$GitHubRepo/releases/download/v$ver/$assetName"
$checksumsUrl = "https://github.com/$GitHubRepo/releases/download/v$ver/SHA256SUMS"

# Download
Push-Location $TmpDir
try {
    Invoke-Download $downloadUrl (Join-Path $TmpDir $assetName)
    try { Invoke-Download $checksumsUrl (Join-Path $TmpDir "SHA256SUMS") } catch { Write-Warn "SHA256SUMS download failed" }
    Verify-Checksum $assetName (Join-Path $TmpDir "SHA256SUMS")

    Write-Step "Extracting archive..."
    Expand-Zip (Join-Path $TmpDir $assetName) $TmpDir
} finally {
    Pop-Location
}

# Determine install dir
if ($env:OKF_INSTALL_DIR) {
    $installDir = $env:OKF_INSTALL_DIR
} else {
    $installDir = Join-Path $env:USERPROFILE "bin"
}
New-Item -ItemType Directory -Path $installDir -Force | Out-Null

# Locate binary
$binary = Get-ChildItem -Path $TmpDir -Filter "$BinaryName.exe" -Recurse | Select-Object -First 1 -ExpandProperty FullName
if (-not $binary -or -not (Test-Path $binary)) {
    Write-Err "Could not find $BinaryName.exe in archive"
    exit 1
}

# Install
$target = Join-Path $installDir "$BinaryName.exe"
Copy-Item $binary $target -Force
Write-Ok "Installed okf.exe to $target"

# PATH check / addition
if ($env:PATH -notlike "*$installDir*") {
    Write-Warn "$installDir is not in your PATH."
    $choice = Read-Host "Add it to your user PATH? (y/N)"
    if ($choice -eq "y" -or $choice -eq "Y") {
        $current = [Environment]::GetEnvironmentVariable("Path", "User")
        if ($current -notlike "*$installDir*") {
            [Environment]::SetEnvironmentVariable("Path", "$current;$installDir", "User")
            $env:PATH = "$env:PATH;$installDir"
            Write-Ok "Added $installDir to user PATH (restart your shell for changes to take effect)"
        }
    } else {
        Write-Step "Add it manually to your PATH: $installDir"
    }
}

Write-Host ""
Write-Host "Installation complete!" -ForegroundColor Green
Write-Host "  Run:  okf.exe --help"
Write-Host ""
