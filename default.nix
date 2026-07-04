{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.13";
  src = ./.;
  vendorHash = "sha256-QBqYyao/Q5x2+m1H129x/n/uKdpVFQsFrgUtQcpZJ7s=";
}
