# mega-speak — the CLI and the speak server; python3 only. The version is
# the flake's revision, printed by `mega-speak --version`.
{
  lib,
  stdenvNoCC,
  python3,
  rev ? "dev",
}:
stdenvNoCC.mkDerivation rec {
  pname = "mega-speak";
  version = "0-${rev}";
  src = lib.fileset.toSource {
    root = ./.;
    fileset = ./bin;
  };
  buildInputs = [ python3 ]; # patchShebangs points the scripts at it
  installPhase = ''
    install -Dm755 bin/mega-speak $out/bin/mega-speak
    install -Dm755 bin/mega-speak-server $out/bin/mega-speak-server
    substituteInPlace $out/bin/mega-speak --replace-fail '@version@' '${version}'
  '';
  meta = {
    description = "Say things out loud with Volcengine seed-tts-2.0, here or on another host's speaker";
    mainProgram = "mega-speak";
    platforms = lib.platforms.unix;
  };
}
