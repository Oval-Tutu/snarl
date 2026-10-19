# EngineLab source at a pinned commit, for the profile import (README.md).
# Build with: nix build -f tools/import/enginelab.nix
# Uses the nixpkgs revision pinned in the repository flake.lock. The import
# reads the engine and exhaust YAML files only. It builds and runs nothing
# from this source.
{
  pkgs ?
    let
      lock = (builtins.fromJSON (builtins.readFile ../../flake.lock)).nodes.nixpkgs.locked;
    in
    import (fetchTarball {
      url = "https://github.com/${lock.owner}/${lock.repo}/archive/${lock.rev}.tar.gz";
      sha256 = lock.narHash;
    }) { },
}:
pkgs.fetchFromGitHub {
  owner = "zolaski333";
  repo = "EngineLab";
  # main, 18 October 2026.
  rev = "d345ba42b8201367c91b0f1dcf83cf283af4caec";
  hash = "sha256-jmFX4DpweDrOVGutyOLjYzsCUsDuvuc00y4yPoV6j/0=";
}
