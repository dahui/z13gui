class Z13gui < Formula
  desc "GTK4 overlay GUI companion for z13ctl"
  homepage "https://github.com/dahui/z13gui"
  url "https://github.com/dahui/z13gui/releases/download/vVERSION_PLACEHOLDER/z13gui_VERSION_PLACEHOLDER_linux_amd64.tar.gz"
  sha256 "SHA256_PLACEHOLDER"
  license "Apache-2.0"

  depends_on :linux
  depends_on "z13ctl"

  def install
    bin.install "z13gui"
  end

  def caveats
    <<~EOS
      z13gui requires GTK4 and gtk4-layer-shell installed via your system
      package manager:

        Arch:          sudo pacman -S gtk4 gtk4-layer-shell
        Debian/Ubuntu: sudo apt install libgtk-4-1 libgtk4-layer-shell0
        Fedora:        sudo dnf install gtk4 gtk4-layer-shell

      After installation, enable the z13gui service:
        systemctl --user enable --now z13gui
    EOS
  end

  test do
    system "#{bin}/z13gui", "--help"
  end
end
