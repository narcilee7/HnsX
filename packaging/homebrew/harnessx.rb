# HarnessX Homebrew formula.
#
# Install:
#   brew tap harnessx-ai/tap
#   brew install harnessx
#
# This tap ships three binaries (control plane, CLI, daemon) plus the
# forked Multica binaries used by HarnessX under the hood.

class Harnessx < Formula
  desc "HarnessX — managed agents platform that wraps SOTA Coding Agents with Harness governance"
  homepage "https://harnessx.ai"
  version "1.0.0"

  on_macos do
    on_arm do
      url "https://github.com/harnessx-ai/harnessx/releases/download/v#{version}/harnessx-macos-arm64.tar.gz"
      sha256 "PLACEHOLDER_ARM64_SHA256"
    end
    on_intel do
      url "https://github.com/harnessx-ai/harnessx/releases/download/v#{version}/harnessx-macos-amd64.tar.gz"
      sha256 "PLACEHOLDER_AMD64_SHA256"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/harnessx-ai/harnessx/releases/download/v#{version}/harnessx-linux-arm64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_ARM64_SHA256"
    end
    on_intel do
      url "https://github.com/harnessx-ai/harnessx/releases/download/v#{version}/harnessx-linux-amd64.tar.gz"
      sha256 "PLACEHOLDER_LINUX_AMD64_SHA256"
    end
  end

  depends_on "postgresql@16"

  def install
    bin.install "harnessx"
    bin.install "hnsx-server"
    bin.install "harnessx-daemon"
    bin.install "multica"
    bin.install "multica-server"

    # Auto-launchd plist for macOS: starts the server on user login.
    if OS.mac?
      (prefix/"com.harnessx.server.plist").write(launchd_plist)
    end
  end

  def post_install
    # First-run setup: create data dir, prompt for secret key.
    ohai "HarnessX installed. Run \`harnessx setup\` to initialize."
    ohai "To start the server now: \`harnessx-server server\`"
    ohai "To install the daemon:   \`harnessx-daemon --server http://127.0.0.1:50051\`"
  end

  def caveats
    <<~EOS
      HarnessX bundles a forked Multica backend (multica-server). Both
      expose the same /api/* contract; the HarnessX daemon connects to
      either one. For most installs:

        # Start the control plane (Postgres-backed, single binary)
        harnessx-server server

        # In another shell, start the local daemon
        harnessx-daemon --server http://127.0.0.1:50051
    EOS
  end

  def launchd_plist
    <<~XML
      <?xml version="1.0" encoding="UTF-8"?>
      <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
      <plist version="1.0">
      <dict>
        <key>Label</key><string>com.harnessx.server</string>
        <key>ProgramArguments</key>
        <array>
          <string>#{opt_bin}/harnessx-server</string>
          <string>server</string>
        </array>
        <key>RunAtLoad</key><true/>
        <key>KeepAlive</key><true/>
        <key>StandardOutPath</key><string>#{var}/log/harnessx-server.log</string>
        <key>StandardErrorPath</key><string>#{var}/log/harnessx-server.log</string>
      </dict>
      </plist>
    XML
  end

  test do
    system bin/"harnessx-server", "--help"
    system bin/"harnessx-daemon", "--help"
  end
end
