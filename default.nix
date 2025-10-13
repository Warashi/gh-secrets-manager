{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.9";
  src = ./.;
  vendorHash = "sha256-df7JalcSJLIiIBuWfnXUpLKUK1xjqOtNAGEm9d2VKhY=";
}
