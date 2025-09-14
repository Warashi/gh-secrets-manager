{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.8";
  src = ./.;
  vendorHash = "sha256-5035ags3XSLuEShCFxUk6WOp5MTSPb1oLak+5O3XaXc=";
}
