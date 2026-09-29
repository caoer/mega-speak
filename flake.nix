{
  description = "Mega Speak: say things out loud with Volcengine seed-tts-2.0, and a macOS menu bar list of what was said";

  inputs.nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "aarch64-darwin"
        "x86_64-darwin"
        "aarch64-linux"
        "x86_64-linux"
      ];
      rev = self.shortRev or self.dirtyShortRev or "dev";
      forAll = f: nixpkgs.lib.genAttrs systems (s: f nixpkgs.legacyPackages.${s});
    in
    {
      packages = forAll (
        pkgs:
        {
          mega-speak = pkgs.callPackage ./package.nix { inherit rev; };
          default = self.packages.${pkgs.stdenv.hostPlatform.system}.mega-speak;
        }
        // nixpkgs.lib.optionalAttrs pkgs.stdenv.hostPlatform.isDarwin {
          mega-speak-app = pkgs.callPackage ./app.nix { inherit rev; };
        }
      );
    };
}
