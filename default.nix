{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.7";
  src = ./.;
  vendorHash = "sha256-5403cM8Ef1A0riai/Bsz0qzqnyaGDEFIyVWpl9loY0Y=";
}
