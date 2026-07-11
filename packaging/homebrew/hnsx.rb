# Homebrew formula scaffold for HnsX.
#
# Published as hnsx-io/homebrew-hnsx/tap/hnsx.rb once v1.0 ships. The real
# release pipeline updates the `url` / `sha256` / `version` from the GitHub
# release metadata; this file documents the expected shape and pins the
# stable channel.
class Hnsx < Formula
  desc "Operator CLI for the HnsX Harness-as-a-Service platform"
  homepage "https://hnsx.dev"
  version "0.8.0"
  url "https://github.com/hnsx-io/hnsx/releases/download/v#{version}/hnsx_#{version}_#{OS}_#{ARCH}.tar.gz"
  sha256 "REPLACE_AT_RELEASE_TIME"

  depends_on :macos

  def install
    bin.install "hnsx"
  end

  test do
    assert_match "hnsx", shell_output("#{bin}/hnsx version")
  end
end