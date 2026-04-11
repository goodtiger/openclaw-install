class OpenclawInstall < Formula
  desc "OpenClaw installer for China region — network-optimized installation and configuration"
  homepage "https://github.com/goodtiger/openclaw-install"
  version "0.2.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/goodtiger/openclaw-install/releases/download/v0.2.0/openclaw-install_0.2.0_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_DARWIN_ARM64"
    end
    on_intel do
      url "https://github.com/goodtiger/openclaw-install/releases/download/v0.2.0/openclaw-install_0.2.0_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_DARWIN_AMD64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/goodtiger/openclaw-install/releases/download/v0.2.0/openclaw-install_0.2.0_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_ARM64"
    end
    on_intel do
      url "https://github.com/goodtiger/openclaw-install/releases/download/v0.2.0/openclaw-install_0.2.0_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_AMD64"
    end
  end

  livecheck do
    url :stable
    regex(/^v?(\d+(?:\.\d+)*)$/i)
  end

  def install
    bin.install "openclaw-install"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/openclaw-install version")
  end
end
