{
  description = "Go and Ebitengine development environment for engine audio";

  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";

  outputs =
    { nixpkgs, ... }:
    let
      forAllSystems = nixpkgs.lib.genAttrs [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
    in
    {
      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          nativeLibraries = with pkgs; [
            alsa-lib
            libGL
            libX11
            libXcursor
            libXext
            libXi
            libXinerama
            libXrandr
            libXrender
            libXxf86vm
          ];
        in
        {
          default = pkgs.mkShell {
            packages =
              with pkgs;
              [
                go_1_27
                gopls
                just
                pkg-config
                python3
              ]
              ++ lib.optionals stdenv.hostPlatform.isLinux [ xvfb-run ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux nativeLibraries;
            shellHook = pkgs.lib.optionalString pkgs.stdenv.hostPlatform.isLinux ''
              export LD_LIBRARY_PATH="${pkgs.lib.makeLibraryPath nativeLibraries}''${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
            '';
          };
        }
      );

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
