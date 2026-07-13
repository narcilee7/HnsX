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
    # Auto-launch hnsx-server via launchd on install. The plist template
    # is part of the Homebrew formula source; we expand the placeholders
    # for the user's home + the prefix hnsx-server lives in.
    agents_dir = File.expand_path("~/Library/LaunchAgents")
    FileUtils.mkdir_p(agents_dir)
    plist_src = buildpath/"deployments/launchd/com.narcilee7.hnsx-server.plist"
    plist_dst = "#{agents_dir}/com.narcilee7.hnsx-server.plist"
    data_dir  = File.expand_path("~/.local/share/hnsx")
    log_dir   = File.expand_path("~/.local/var/log")
    FileUtils.mkdir_p(data_dir)
    FileUtils.mkdir_p(log_dir)
    rendered = File.read(plist_src)
      .gsub("__PREFIX__", HOMEBREW_PREFIX.to_s)
      .gsub("__DATA_DIR__", data_dir)
      .gsub("__LOG_DIR__", log_dir)
    File.write(plist_dst, rendered)
    system "launchctl", "load", "-w", plist_dst
    ohai "hnsx-server launched via launchd at #{plist_dst}"
  end

  def uninstall
    plist = File.expand_path("~/Library/LaunchAgents/com.narcilee7.hnsx-server.plist")
    if File.exist?(plist)
      system "launchctl", "unload", "-w", plist
      File.delete(plist)
    end
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/hnsx version")
  end
end
