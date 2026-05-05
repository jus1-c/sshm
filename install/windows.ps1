# SSHM Windows Installation Script
# Usage:
#   Online:  irm https://raw.githubusercontent.com/jus1-c/sshm/main/install/windows.ps1 | iex
#   Version: iex "& { $(irm https://raw.githubusercontent.com/jus1-c/sshm/main/install/windows.ps1) } -Version v1.8.0"
#   Local:   .\install\windows.ps1 -LocalBinary ".\sshm.exe"

param(
    [string]$InstallDir = "$env:LOCALAPPDATA\sshm",
    [switch]$Force = $false,
    [string]$Version = "latest",
    [string]$LocalBinary = ""
)

$ErrorActionPreference = "Stop"
$Repo = "jus1-c/sshm"

function Write-ColorOutput {
    param(
        [ConsoleColor]$ForegroundColor,
        [Parameter(ValueFromRemainingArguments = $true)]
        [object[]]$Message
    )

    $fc = $host.UI.RawUI.ForegroundColor
    $host.UI.RawUI.ForegroundColor = $ForegroundColor
    try {
        if ($Message) {
            Write-Host ($Message -join " ")
        }
    } finally {
        $host.UI.RawUI.ForegroundColor = $fc
    }
}

function Write-Info { Write-ColorOutput Green @args }
function Write-Warn { Write-ColorOutput Yellow @args }
function Write-Fail { Write-ColorOutput Red @args }

function Get-TargetVersion {
    if ($Version -eq "latest") {
        Write-Info "Fetching latest version..."
        try {
            $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
            return $release.tag_name
        } catch {
            Write-Fail "Failed to fetch latest version information"
            exit 1
        }
    }

    Write-Info "Using specified version: $Version"
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/tags/$Version"
        return $release.tag_name
    } catch {
        Write-Fail "Version $Version was not found"
        exit 1
    }
}

function Get-ReleaseArch {
    if ([Environment]::Is64BitOperatingSystem) {
        return "x86_64"
    }
    return "i386"
}

Write-Info "Installing SSHM - SSH Manager"
Write-Info ""

$targetPath = Join-Path $InstallDir "sshm.exe"

$existingSSHM = Get-Command sshm -ErrorAction SilentlyContinue
if ($existingSSHM -and -not $Force) {
    $currentVersion = & sshm --version 2>$null | Select-String "version" | ForEach-Object { $_.ToString().Split()[-1] }
    Write-Warn "SSHM is already installed (version: $currentVersion)"
    $response = Read-Host "Do you want to continue with the installation? (y/N)"
    if ($response -ne "y" -and $response -ne "Y") {
        Write-Info "Installation cancelled."
        exit 0
    }
}

$releaseArch = Get-ReleaseArch
Write-Info "Detected platform: Windows ($releaseArch)"

if (-not (Test-Path $InstallDir)) {
    Write-Info "Creating installation directory: $InstallDir"
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

if ($LocalBinary -ne "") {
    if (-not (Test-Path $LocalBinary)) {
        Write-Fail "Local binary not found: $LocalBinary"
        exit 1
    }

    Write-Info "Using local binary: $LocalBinary"
    Write-Info "Installing binary to: $targetPath"
    Copy-Item -Path $LocalBinary -Destination $targetPath -Force
} else {
    $targetVersion = Get-TargetVersion
    Write-Info "Target version: $targetVersion"

    $fileName = "sshm_Windows_$releaseArch.zip"
    $downloadUrl = "https://github.com/$Repo/releases/download/$targetVersion/$fileName"
    $tempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("sshm-install-" + [System.Guid]::NewGuid().ToString())
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    try {
        $tempFile = Join-Path $tempDir $fileName

        Write-Info "Downloading $fileName..."
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tempFile

        Write-Info "Extracting..."
        Expand-Archive -Path $tempFile -DestinationPath $tempDir -Force
        $extractedBinary = Get-ChildItem -Path $tempDir -Filter "sshm.exe" -Recurse | Select-Object -First 1
        if (-not $extractedBinary) {
            Write-Fail "Could not find sshm.exe in the release archive"
            exit 1
        }

        Write-Info "Installing binary to: $targetPath"
        Move-Item -Path $extractedBinary.FullName -Destination $targetPath -Force
    } finally {
        if (Test-Path $tempDir) {
            Remove-Item $tempDir -Recurse -Force
        }
    }
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$pathEntries = @()
if (-not [string]::IsNullOrWhiteSpace($userPath)) {
    $pathEntries = $userPath -split ";" | Where-Object { $_ }
}

if ($pathEntries -notcontains $InstallDir) {
    Write-Warn "The directory $InstallDir is not in your PATH."
    Write-Info "Adding to user PATH..."
    $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    Write-Info "Please restart your terminal to use the 'sshm' command."
}

Write-Info ""
Write-Info "SSHM successfully installed to: $targetPath"
Write-Info "You can now use the 'sshm' command!"

if (Test-Path $targetPath) {
    Write-Info ""
    Write-Info "Verifying installation..."
    & $targetPath --version
}
