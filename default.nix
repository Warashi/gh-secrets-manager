{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  name = "gh-secrets-manager";
  version = "0.0.2";
  src = ./.;
  vendorHash = "sha256-LJdpcXW738We5RH+wKkN8UdRlGlg7It1LZCYFVSPl4U=";
}
