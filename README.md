# idasen

`idasen` controls IKEA IDÅSEN desks and compatible Linak controllers over
Bluetooth Low Energy. It includes a CLI, a portable system tray menu, a
streaming controller API, and a native DankMaterialShell widget.

The Linux implementation uses BlueZ over D-Bus. It does not require root or
Linux network capabilities.

Inspired by [newAM/idasen](https://github.com/newAM/idasen/).

<img width="786" height="804" alt="1784104509430179754" src="https://github.com/user-attachments/assets/9a535184-6cf1-4b45-9804-dd05a9bbf734" />


## Features

- Read the current height and movement speed.
- Monitor height changes continuously.
- Move to an exact height or a named saved position.
- Save and delete any number of named positions.
- Portable StatusNotifierItem tray menu.
- Native DankMaterialShell bar widget and persistent popout.
- Live height feedback while the desk moves.
- Sit, Stand, Stop, and configurable fine-adjustment controls.
- Editable Sit/Stand heights from the DMS popout.
- Long-running newline-delimited JSON integration API.
- Nix flake package, overlay, and Home Manager module.

The supported desk range is 62–127 cm.

## Pair and configure

Pair the desk using your desktop Bluetooth settings or `bluetoothctl`. Pairing
is intentionally left to the operating system.

Create the initial configuration and discover a nearby desk:

```sh
idasen init
```

The configuration is stored at `$XDG_CONFIG_HOME/idasen/idasen.yaml`, falling
back to `~/.config/idasen/idasen.yaml`:

```yaml
mac_address: AA:AA:AA:AA:AA:AA
positions:
  sit: 0.75
  stand: 1.10
```

If discovery does not find the desk, enter its Bluetooth MAC address manually.
All heights in the configuration and CLI are expressed in metres.

## Command line

```sh
# One-off readings
idasen height
idasen speed

# Print height and speed whenever they change
idasen monitor

# Move to 90 cm
idasen move 0.90

# Move to a configured named position
idasen sit
idasen stand

# Save the current height or delete a saved position
idasen save focus
idasen delete focus

# Graphical integrations
idasen tray
idasen controller
```

Pass the MAC address before a command to override the configuration:

```sh
idasen --mac-address AA:AA:AA:AA:AA:AA height
```

Movement stops when the target is reached, the desk stalls, the two-minute
movement timeout expires, or the process is interrupted.

## System tray

`idasen tray` publishes a StatusNotifierItem containing the current height and
all saved positions. The desktop shell renders the menu in its native style.

The tray is the portable interface. Standard DBus menus cannot contain numeric
inputs or control whether the shell closes them after an action; use the DMS
widget for the full interactive experience.

## DankMaterialShell widget

The plugin in `dms/Idasen` integrates directly with DankBar. Its popout provides:

- live height and movement state;
- Sit and Stand buttons that leave the popout open;
- Stop and fine Up/Down adjustment buttons;
- editable Sit and Stand heights; and
- connection and controller error reporting.

The adjustment step defaults to 5 mm and is configurable in the plugin settings.
Edited positions are written to the same `idasen.yaml` used by the CLI.

The widget runs `idasen controller` as a child process. That process owns one
persistent BLE connection, avoiding reconnect latency for every button press.

After installing the plugin, open DMS Settings, scan for plugins, enable
**IDÅSEN Desk**, and add it to the DankBar layout.

## Nix flake

The flake supports `x86_64-linux` and `aarch64-linux` and exposes:

- `packages.<system>.default` — the `idasen` executable plus DMS plugin;
- `overlays.default` — adds `pkgs.go-idasen`; and
- `homeManagerModules.default` — installs both declaratively.

Run directly from the repository:

```sh
nix run github:edofic/go-idasen -- height
```

For a declarative Home Manager setup, add the flake input and module:

```nix
{
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    home-manager.url = "github:nix-community/home-manager";
    go-idasen.url = "github:edofic/go-idasen";
  };

  outputs = { nixpkgs, home-manager, go-idasen, ... }: {
    homeConfigurations.andraz = home-manager.lib.homeManagerConfiguration {
      pkgs = nixpkgs.legacyPackages.x86_64-linux;
      modules = [
        go-idasen.homeManagerModules.default
        {
          programs.go-idasen.enable = true;
        }
      ];
    };
  };
}
```

Available Home Manager options:

```nix
programs.go-idasen = {
  enable = true;
  package = inputs.go-idasen.packages.${pkgs.system}.default;
  installDmsPlugin = true;
};
```

The Nix-packaged widget calls the exact `idasen` binary in the Nix store, so it
does not depend on the DMS service's `PATH`. Home Manager creates the plugin link
under `~/.config/DankMaterialShell/plugins/Idasen` during activation.

If replacing a manual installation, remove the old unmanaged files once before
the first Home Manager activation:

```sh
rm -f ~/.local/bin/idasen
rm -f ~/.config/DankMaterialShell/plugins/Idasen
home-manager switch --flake .#andraz
```

You can also consume the overlay without the Home Manager module:

```nix
nixpkgs.overlays = [ inputs.go-idasen.overlays.default ];
environment.systemPackages = [ pkgs.go-idasen ];
```

In that case, install the DMS plugin separately from
`${pkgs.go-idasen}/share/dankmaterialshell/plugins/Idasen`.

## Build from source

With Go 1.24 or newer:

```sh
go build -buildvcs=false -o idasen .
go test ./...
```

For a non-Nix DMS installation:

```sh
go build -buildvcs=false -o ~/.local/bin/idasen .
mkdir -p ~/.config/DankMaterialShell/plugins
ln -s "$PWD/dms/Idasen" ~/.config/DankMaterialShell/plugins/Idasen
```

The source plugin defaults to `~/.local/bin/idasen`; this can be changed in its
DMS settings.

## Controller protocol

`idasen controller` is intended for graphical integrations. It keeps the desk
connected, emits one JSON object per line on stdout, and accepts one JSON command
per line on stdin.

Commands include:

```json
{"action":"move","position":"sit"}
{"action":"move","height":0.90}
{"action":"adjust","delta":0.005}
{"action":"set_position","position":"stand","height":1.10}
{"action":"refresh"}
{"action":"stop"}
```

Events report `status`, `height`, `positions`, and `error` changes. Heights and
deltas use metres; speed uses metres per second.
