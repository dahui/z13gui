# Installation

## Prerequisites

- Linux kernel (x86_64)
- Wayland compositor with layer-shell support, or gamescope (Steam Gaming Mode)
- [z13ctl](https://github.com/dahui/z13ctl) installed and daemon running

### Runtime dependencies

z13gui links dynamically against GTK 4 and gtk4-layer-shell. The AUR package
pulls these automatically; for other install methods, install them via your
system package manager first.

| Dependency | Arch | Debian / Ubuntu | Fedora |
|---|---|---|---|
| GTK 4 | `gtk4` | `libgtk-4-1` | `gtk4` |
| gtk4-layer-shell | `gtk4-layer-shell` | `libgtk4-layer-shell0` | `gtk4-layer-shell` |

---

## Install

=== "Arch Linux (AUR)"

    ```sh
    paru -S z13gui-bin
    ```

    Or with yay:

    ```sh
    yay -S z13gui-bin
    ```

    The package installs the binary, systemd service, udev rules, and desktop entry.
    Services are enabled automatically for all users on next login.

=== "Linuxbrew"

    ```sh
    brew install dahui/z13ctl/z13gui
    ```

    Then enable the systemd service:

    ```sh
    systemctl --user enable --now z13gui
    ```

=== "Release binary"

    Install the [runtime dependencies](#runtime-dependencies) for your distro,
    then download the latest `linux_amd64` archive from the
    [Releases](https://github.com/dahui/z13gui/releases) page:

    ```sh
    tar xzf z13gui_*_linux_amd64.tar.gz
    sudo install -Dm755 z13gui /usr/local/bin/z13gui
    ```

    Install the systemd user service:

    ```sh
    install -Dm644 contrib/z13gui.service \
        ~/.config/systemd/user/z13gui.service
    systemctl --user daemon-reload
    systemctl --user enable --now z13gui
    ```

    Optionally install the desktop entry:

    ```sh
    install -Dm644 contrib/z13gui.desktop \
        ~/.local/share/applications/z13gui.desktop
    ```

=== "From source"

    Requires Go 1.23+, CGO enabled, and GTK4 development libraries.

    **Arch Linux:**

    ```sh
    sudo pacman -S gtk4 gtk4-layer-shell
    ```

    **Debian/Ubuntu:**

    ```sh
    sudo apt-get install -y libgtk-4-dev libgtk4-layer-shell-dev
    ```

    **Fedora:**

    ```sh
    sudo dnf install gtk4-devel gtk4-layer-shell-devel
    ```

    Then clone and build:

    ```sh
    git clone https://github.com/dahui/z13gui
    cd z13gui
    make build
    sudo make install
    make install-service
    ```

---

## Verify the installation

```sh
z13gui --version
```

Then press the Armoury Crate button on your Z13. The drawer should slide in
from the right edge of the screen.

---

## Uninstall

Stop and remove the service:

```sh
make uninstall-service
```

Or manually:

```sh
systemctl --user disable --now z13gui
rm -f ~/.config/systemd/user/z13gui.service
systemctl --user daemon-reload
```

Remove the binary:

```sh
sudo rm /usr/local/bin/z13gui
```

---

## Troubleshooting

**Drawer doesn't appear**

Make sure the z13ctl daemon is running:

```sh
systemctl --user status z13ctl.service
```

**Service fails to start**

Check the journal:

```sh
journalctl --user -u z13gui -n 50
```

Run with debug logging to see GTK and initialization output:

```sh
z13gui --debug
```

**Gamescope: drawer doesn't show**

Verify `GAMESCOPE_WAYLAND_DISPLAY` is set and the socket exists:

```sh
echo $GAMESCOPE_WAYLAND_DISPLAY
ls "$XDG_RUNTIME_DIR/$GAMESCOPE_WAYLAND_DISPLAY"
```

If the socket is missing (stale environment from a previous Gaming Mode session),
z13gui automatically falls back to Wayland layer-shell mode.
