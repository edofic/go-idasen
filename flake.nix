{
  description = "Go controller and DankMaterialShell widget for IKEA IDÅSEN desks";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [
        "aarch64-linux"
        "x86_64-linux"
      ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = pkgs.buildGoModule {
            pname = "go-idasen";
            version = "0.2.0";
            src = self;

            vendorHash = "sha256-dx5NNBZXJSCODM5LZMPpnAflPkJeEb5QIZZOppRRGbM=";

            subPackages = [ "." ];

            postInstall = ''
              mv $out/bin/go-idasen $out/bin/idasen

              install -Dm644 dms/Idasen/plugin.json \
                $out/share/dankmaterialshell/plugins/Idasen/plugin.json
              install -Dm644 dms/Idasen/IdasenWidget.qml \
                $out/share/dankmaterialshell/plugins/Idasen/IdasenWidget.qml
              install -Dm644 dms/Idasen/IdasenSettings.qml \
                $out/share/dankmaterialshell/plugins/Idasen/IdasenSettings.qml

              substituteInPlace \
                $out/share/dankmaterialshell/plugins/Idasen/IdasenWidget.qml \
                $out/share/dankmaterialshell/plugins/Idasen/IdasenSettings.qml \
                --replace-fail '@IDASEN_BINARY@' "$out/bin/idasen"
            '';

            meta = {
              description = "Controller and DMS widget for IKEA IDÅSEN desks";
              homepage = "https://github.com/edofic/go-idasen";
              mainProgram = "idasen";
              platforms = nixpkgs.lib.platforms.linux;
            };
          };
        }
      );

      overlays.default = final: _prev: {
        go-idasen = self.packages.${final.system}.default;
      };

      homeManagerModules.default =
        {
          config,
          lib,
          pkgs,
          ...
        }:
        let
          cfg = config.programs.go-idasen;
        in
        {
          options.programs.go-idasen = {
            enable = lib.mkEnableOption "go-idasen desk controller";

            package = lib.mkOption {
              type = lib.types.package;
              default = self.packages.${pkgs.system}.default;
              defaultText = lib.literalExpression "inputs.go-idasen.packages.${pkgs.system}.default";
              description = "The go-idasen package to install.";
            };

            installDmsPlugin = lib.mkOption {
              type = lib.types.bool;
              default = true;
              description = "Install the DankMaterialShell widget plugin.";
            };
          };

          config = lib.mkIf cfg.enable {
            home.packages = [ cfg.package ];

            xdg.configFile."DankMaterialShell/plugins/Idasen" = lib.mkIf cfg.installDmsPlugin {
              source = "${cfg.package}/share/dankmaterialshell/plugins/Idasen";
            };
          };
        };
    };
}
