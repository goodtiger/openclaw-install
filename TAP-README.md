# Homebrew Tap for openclaw-install

Official Homebrew tap for [openclaw-install](https://github.com/goodtiger/openclaw-install) — the OpenClaw installer optimized for China region networks.

## Installation

```bash
# Tap the repository
brew tap openmule/tap

# Install openclaw-install
brew install openclaw-install
```

Or install directly without tapping:

```bash
brew install openmule/tap/openclaw-install
```

## Usage

```bash
# Check version
openclaw-install version

# Run environment diagnostics
openclaw-install doctor

# Interactive installation
openclaw-install install

# Quick installation (non-interactive)
openclaw-install install --yes --provider bailian --api-key sk-xxx
```

## Updating

```bash
brew upgrade openclaw-install
```

## Supported Platforms

| OS | Architecture |
|----|-------------|
| macOS | arm64 (Apple Silicon) |
| macOS | amd64 (Intel) |
| Linux | arm64 |
| Linux | amd64 |

## How It Works

This tap hosts a Homebrew formula that downloads precompiled binaries from the [GitHub Releases](https://github.com/goodtiger/openclaw-install/releases) page. When a new release is published, a GitHub Action automatically updates the formula with the correct version and SHA256 checksums.

## License

Same as [openclaw-install](https://github.com/goodtiger/openclaw-install).
