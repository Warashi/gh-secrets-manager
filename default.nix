{
  pkgs ? import <nixpkgs> { },
}:
pkgs.buildGoLatestModule {
  pname = "gh-secrets-manager";
  version = "0.0.4";
  src = ./.;
  vendorHash = "sha256-qKthz37Tu8SrDV0iA+nHWVRzeOmwNcZjKbBbRenP/4Q=";
}
