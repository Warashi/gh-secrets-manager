{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.3";
  src = ./.;
  vendorHash = "sha256-LJdpcXW738We5RH+wKkN8UdRlGlg7It1LZCYFVSPl4U=";
}
