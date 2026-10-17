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
          # The toolchain line of go.mod must name this version.
          go = pkgs.go_1_27;
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
                # ffmpeg measures loudness and spectra of the renders.
                ffmpeg-headless
                gopls
                just
                pkg-config
                python3
              ]
              ++ lib.optionals stdenv.hostPlatform.isLinux [ xvfb-run ]
              # Tailor picks the default go, so give its package list the Go of the shell in its place.
              ++ import ./nix/loader.nix {
                pkgs = pkgs // {
                  inherit go;
                };
              };
            buildInputs = pkgs.lib.optionals pkgs.stdenv.hostPlatform.isLinux nativeLibraries;
            shellHook =
              import ./nix/hooks.nix { inherit pkgs; }
              + pkgs.lib.optionalString pkgs.stdenv.hostPlatform.isLinux ''

                tailor_prepend_path LD_LIBRARY_PATH ${pkgs.lib.makeLibraryPath nativeLibraries}
              '';
          };
        }
      );

      formatter = forAllSystems (system: nixpkgs.legacyPackages.${system}.nixfmt);
    };
}
