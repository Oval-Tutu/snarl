{
  lib,
  clangStdenv,
  fetchFromGitHub,
  cmake,
  ninja,
  flex,
  bison,
  boost,
}:

let
  # Upstream master head, 22 January 2023. The delta-studio and
  # direct-to-video submodules are skipped: the headless build does not
  # use them, and delta-studio is about 800 MB.
  engine-sim = fetchFromGitHub {
    owner = "ange-yaghi";
    repo = "engine-sim";
    rev = "85f7c3b959a908ed5232ede4f1a4ac7eafe6b630";
    hash = "sha256-+nUnz32GlInAOH1luw3LjYJeM2mz9CVhfq+xk1+PavA=";
  };
  submodules = {
    simple-2d-constraint-solver = fetchFromGitHub {
      owner = "ange-yaghi";
      repo = "simple-2d-constraint-solver";
      rev = "e009f4ff1c9c4c5874e865e893cdb62e208fb2b3";
      hash = "sha256-A1qhgBQFj4kk/UsHYM/nQIoiFqfBb10fPhhjCqXTm0U=";
    };
    csv-io = fetchFromGitHub {
      owner = "ange-yaghi";
      repo = "csv-io";
      rev = "2112c55e1e831c8f7a1b91de23722b7926ad3d00";
      hash = "sha256-fE3p6TAx4eBxVWC/QtywwuB3kZsmfVM5KVprR29IEUs=";
    };
    piranha = fetchFromGitHub {
      owner = "ange-yaghi";
      repo = "piranha";
      rev = "432f0b122bb1663b686c553c7e7269300afac3bc";
      hash = "sha256-3t7RCOX6uZy+pr1RnFKWFYUhBVAE/gYhRQGRPgLX2ks=";
    };
  };
in
# Clang, not GCC: upstream and piranha use explicit specialisations in
# class scope (an MSVC extension that Clang accepts and GCC rejects).
clangStdenv.mkDerivation {
  pname = "engine-sim-render";
  version = "0-unstable-2023-01-22";

  driver = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./CMakeLists.txt
      ./engine_sim_render.cpp
    ];
  };

  unpackPhase = ''
    runHook preUnpack
    cp -r $driver driver
    cp -r ${engine-sim} engine-sim
    chmod -R u+w driver engine-sim
    ${lib.concatStrings (
      lib.mapAttrsToList (name: src: ''
        rm -rf engine-sim/dependencies/submodules/${name}
        cp -r ${src} engine-sim/dependencies/submodules/${name}
      '') submodules
    )}
    chmod -R u+w engine-sim
    runHook postUnpack
  '';

  patches = lib.filesystem.listFilesRecursive ./patches;
  patchFlags = [
    "-p1"
    "-d"
    "engine-sim"
  ];

  nativeBuildInputs = [
    cmake
    ninja
    flex
    bison
  ];
  buildInputs = [ boost ];

  cmakeDir = "../driver";
  preConfigure = ''
    cmakeFlagsArray+=("-DENGINE_SIM_SOURCE_DIR=$PWD/engine-sim")
  '';

  meta = {
    description = "Headless engine-sim renderer for snarl reference audio";
    homepage = "https://github.com/ange-yaghi/engine-sim";
    license = lib.licenses.mit;
    mainProgram = "engine-sim-render";
    platforms = lib.platforms.linux;
  };
}
