{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.13";
  src = ./.;
  vendorHash = "sha256-dGjVOIZY7P9eMv94v6Vd2BuUASLpsrnjw6ixi0jyfd8=";
}
