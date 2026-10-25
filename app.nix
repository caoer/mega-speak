# mega-speak-app — Mega Speak's menu bar app (cmd/mega-speak-bar); cgo-free,
# purego drives AppKit. The unsigned MegaSpeak.app and install-app.sh are in
# libexec/mega-speak/; install-app.sh signs it into ~/Applications.
{
  lib,
  buildGo127Module,
  rev ? "dev",
}:
buildGo127Module rec {
  pname = "mega-speak-app";
  version = "0-${rev}";
  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./go.sum
      ./cmd
      ./internal
      ./packaging
      ./scripts
      ./vendor
    ];
  };
  vendorHash = null; # vendor/: mega-update is a private module the build sandbox cannot fetch
  subPackages = [ "cmd/mega-speak-bar" ];
  env.CGO_ENABLED = "0";
  ldflags = [
    "-s"
    "-w"
    "-X main.version=${version}"
  ];
  postInstall = ''
    app=$out/libexec/mega-speak/MegaSpeak.app
    install -Dm755 $out/bin/mega-speak-bar $app/Contents/MacOS/mega-speak-bar
    install -Dm644 packaging/Info.plist $app/Contents/Info.plist
    install -Dm755 scripts/install-app.sh $out/libexec/mega-speak/install-app.sh
  '';
  meta = {
    description = "Mega Speak's menu bar list of what was said aloud";
    mainProgram = "mega-speak-bar";
    platforms = lib.platforms.darwin;
  };
}
