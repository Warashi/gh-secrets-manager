{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.6";
  src = ./.;
  vendorHash = "sha256-uo+8aHzvmiIGteF1vwF69D0T8U7/zaOThcdusjmXfto=";
}
