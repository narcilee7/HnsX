# Homebrew formula for HnsX.
#
# Published as narcilee7/homebrew-hnsx/tap/hnsx.rb. The release pipeline updates
# the `url` / `sha256` / `version` from the GitHub release metadata; this file
# is the in-repo source of truth used by `brew install narcilee7/hnsx/hnsx`.
class Hnsx < Formula
  desc "Operator CLI for the HnsX Harness-as-a-Service platform"
  homepage "https://hnsx.dev"
  version "1.0.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/narcilee7/HnsX/releases/download/v#{version}/hnsx_darwin_arm64.tar.gz"
      sha256 "REPLACE_AT_RELEASE_TIME_DARWIN_ARM64"
    else
      url "https://github.com/narcilee7/HnsX/releases/download/v#{version}/hnsx_darwin_amd64.tar.gz"
      sha256 "REPLACE_AT_RELEASE_TIME_DARWIN_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/narcilee7/HnsX/releases/download/v#{version}/hnsx_linux_arm64.tar.gz"
      sha256 "REPLACE_AT_RELEASE_TIME_LINUX_ARM64"
    else
      url "https://github.com/narcilee7/HnsX/releases/download/v#{version}/hnsx_linux_amd64.tar.gz"
      sha256 "REPLACE_AT_RELEASE_TIME_LINUX_AMD64"
    end
  end

  def install
    bin.install "hnsx"
    bin.install "hnsx-server"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/hnsx version")
  end
end
