{
  description = "A simple reverse proxy that appears as a additional node in your Tailscale network";

  inputs = {
    nixpkgs.url = "nixpkgs";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { nixpkgs, flake-utils, self, ... }: 
  flake-utils.lib.eachDefaultSystem (system: let
    pkgs = import nixpkgs { inherit system; };
  in {
    packages = {
      default = pkgs.python3Packages.callPackage ./package.nix {};
      container = pkgs.python3Packages.callPackage ./container.nix {
        inherit self;
      };
    };
  });
}