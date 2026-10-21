# Build with: nix build -f reference/enginelab
# Uses the nixpkgs revision pinned in the repository flake.lock.
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
pkgs.callPackage ./package.nix { }
