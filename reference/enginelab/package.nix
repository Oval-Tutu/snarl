{
  lib,
  stdenv,
  fetchFromGitHub,
  cmake,
  ninja,
  pkg-config,
  alsa-lib,
  fontconfig,
  freetype,
  libx11,
  libxcursor,
  libxext,
  libxinerama,
  libxrandr,
}:

let
  # The same commit as tools/import/enginelab.nix (main, 18 October 2026).
  enginelab = fetchFromGitHub {
    owner = "zolaski333";
    repo = "EngineLab";
    rev = "d345ba42b8201367c91b0f1dcf83cf283af4caec";
    hash = "sha256-jmFX4DpweDrOVGutyOLjYzsCUsDuvuc00y4yPoV6j/0=";
  };
  # The FetchContent_Declare tags in the upstream CMakeLists.txt.
  juce = fetchFromGitHub {
    owner = "juce-framework";
    repo = "JUCE";
    tag = "8.0.13";
    hash = "sha256-TKqW2rsFMAO1HJZ9IFQ7myOzNRScqR0gmLSLQA5Sw28=";
  };
  nlohmann-json = fetchFromGitHub {
    owner = "nlohmann";
    repo = "json";
    tag = "v3.12.0";
    hash = "sha256-cECvDOLxgX7Q9R3IE86Hj9JJUxraDQvhoyPDF03B2CY=";
  };
  yaml-cpp = fetchFromGitHub {
    owner = "jbeder";
    repo = "yaml-cpp";
    tag = "0.8.0";
    hash = "sha256-J87oS6Az1/vNdyXu3L7KmUGWzU0IAkGrGMUUha+xDXI=";
  };
in
stdenv.mkDerivation {
  pname = "enginelab-offline-audio-exporter";
  version = "0.3.1-unstable-2026-10-18";

  src = enginelab;

  nativeBuildInputs = [
    cmake
    ninja
    pkg-config
  ];
  # JUCE needs the ALSA headers for juce_audio_devices, and the X11, FreeType
  # and Fontconfig headers for juceaide, which it builds at configure time.
  buildInputs = [
    alsa-lib
    fontconfig
    freetype
    libx11
    libxcursor
    libxext
    libxinerama
    libxrandr
  ];

  cmakeBuildType = "Release";
  cmakeFlags = [
    "-DENGINELAB_BUILD_TESTS=OFF"
    "-DENGINELAB_ENABLE_WARNINGS_AS_ERRORS=OFF"
    "-DFETCHCONTENT_FULLY_DISCONNECTED=ON"
    "-DFETCHCONTENT_SOURCE_DIR_JUCE=${juce}"
    "-DFETCHCONTENT_SOURCE_DIR_NLOHMANN_JSON=${nlohmann-json}"
    "-DFETCHCONTENT_SOURCE_DIR_YAML_CPP=${yaml-cpp}"
  ];
  # yaml-cpp 0.8.0 uses uint16_t without <cstdint>, which GCC 13 and later
  # reject.
  preConfigure = ''
    cmakeFlagsArray+=("-DCMAKE_CXX_FLAGS=-include cstdint")
  '';

  ninjaFlags = [ "EngineLabOfflineAudioExporter" ];

  # The upstream install rules need the GUI application. Install only the
  # exporter and the data that it reads through --catalog-root.
  installPhase = ''
    runHook preInstall
    install -Dm755 tools/EngineLabOfflineAudioExporter -t $out/bin
    mkdir -p $out/share/enginelab
    cp -r $src/engines $src/parts $src/assets $src/voicing $out/share/enginelab/
    install -Dm644 $src/LICENSE.md $src/THIRD_PARTY_NOTICES.md -t $out/share/doc/enginelab
    runHook postInstall
  '';

  meta = {
    description = "EngineLab offline audio exporter, for snarl reference renders only";
    homepage = "https://github.com/zolaski333/EngineLab";
    # EngineLab is MIT. The binary links JUCE under the AGPLv3.
    license = [
      lib.licenses.mit
      lib.licenses.agpl3Only
    ];
    mainProgram = "EngineLabOfflineAudioExporter";
    platforms = lib.platforms.linux;
  };
}
