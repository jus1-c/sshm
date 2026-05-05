# Installation Scripts

This directory contains installation scripts for SSHM.

By default, the scripts install releases from `jus1-c/sshm`.

## Unix/Linux/macOS

### Quick Install

```bash
curl -sSL https://raw.githubusercontent.com/jus1-c/sshm/main/install/unix.sh | bash
```

When using the pipe method, the installer automatically proceeds if SSHM is already installed. Use `FORCE_INSTALL=false` only when running the script from a terminal and you want the overwrite prompt.

### Options

Install a specific version:

```bash
curl -sSL https://raw.githubusercontent.com/jus1-c/sshm/main/install/unix.sh | SSHM_VERSION=v1.8.0 bash
```

Force install without prompts:

```bash
curl -sSL https://raw.githubusercontent.com/jus1-c/sshm/main/install/unix.sh | FORCE_INSTALL=true bash
```

Install to a custom directory:

```bash
curl -sSL https://raw.githubusercontent.com/jus1-c/sshm/main/install/unix.sh | INSTALL_DIR="$HOME/.local/bin" bash
```

Install from a different GitHub repo:

```bash
curl -sSL https://raw.githubusercontent.com/jus1-c/sshm/main/install/unix.sh | SSHM_REPO=owner/repo bash
```

### Manual Script Run

```bash
curl -O https://raw.githubusercontent.com/jus1-c/sshm/main/install/unix.sh
chmod +x unix.sh
./unix.sh
```

## Windows

### Quick Install

```powershell
irm https://raw.githubusercontent.com/jus1-c/sshm/main/install/windows.ps1 | iex
```

### Options

Install a specific version:

```powershell
iex "& { $(irm https://raw.githubusercontent.com/jus1-c/sshm/main/install/windows.ps1) } -Version v1.8.0"
```

Force install without prompts:

```powershell
iex "& { $(irm https://raw.githubusercontent.com/jus1-c/sshm/main/install/windows.ps1) } -Force"
```

Install to a custom directory:

```powershell
iex "& { $(irm https://raw.githubusercontent.com/jus1-c/sshm/main/install/windows.ps1) } -InstallDir 'C:\tools'"
```

Install a local binary:

```powershell
.\install\windows.ps1 -LocalBinary ".\sshm.exe"
```

## What The Installer Does

1. Detects your OS and architecture.
2. Fetches the requested release from GitHub.
3. Downloads the matching GoReleaser archive.
4. Extracts the `sshm` binary into a temporary directory.
5. Installs it into your target install directory.
6. Verifies the installed binary.

## Supported Platforms

- Linux: x86_64, arm64, armv6, armv7, i386
- macOS: x86_64, arm64
- Windows: x86_64, i386

## Requirements

- Unix/Linux/macOS: `curl`, `tar`, and `sudo` when installing to a protected directory.
- Windows: PowerShell 5+ with `Invoke-WebRequest` and `Expand-Archive`.

## Uninstall

Unix/Linux/macOS:

```bash
sudo rm /usr/local/bin/sshm
```

Windows:

```powershell
Remove-Item "$env:LOCALAPPDATA\sshm\sshm.exe"
```
