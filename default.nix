{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.9";
  src = ./.;
  vendorHash = "sha256-MMexhnQP9r0gao8le3Z42tRaPosheh4Ee1TOeYNnlrw=";
}
