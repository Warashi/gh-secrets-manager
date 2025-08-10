{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.6";
  src = ./.;
  vendorHash = "sha256-PHAhO0IwABY/yM9Jsu6Dnl1nSA0tDRtL6gkeKSyxHKM=";
}
