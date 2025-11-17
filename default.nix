{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.11";
  src = ./.;
  vendorHash = "sha256-dG1bIxl/vu4tQjoq2jiyABCp6Z5Bln0/FTdboTqCqE0=";
}
