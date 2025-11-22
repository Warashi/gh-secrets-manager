{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.11";
  src = ./.;
  vendorHash = "sha256-KqXW3kE3G3oHbQGbjv5eSCp0kypLJeQfkbJ+b70W5pU=";
}
