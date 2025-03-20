{
  inputs = {
    nixpkgs.url = "tarball+https://git.tatikoma.dev/corpix/nixpkgs/archive/v2025-03-16.768365.tar.gz";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { nixpkgs, flake-utils, ... }: let
    eachSystem = flake-utils.lib.eachSystem flake-utils.lib.allSystems;
  in eachSystem
    (arch: let
      pkgs = nixpkgs.legacyPackages.${arch}.pkgs;

      inherit (pkgs)
        buildGoModule
        mkShell
      ;
      inherit (pkgs.lib)
        attrValues
      ;

      goverter = buildGoModule rec {
        pname = "goverter";
        version = "1.5.1";

        src = pkgs.fetchFromGitHub {
          owner = "jmattheis";
          repo = "goverter";
          rev = "v${version}";
          hash = "sha256-unmLOwXSexokYulP5DzNjVcUetvcBAJ6XK5zCgHuZQ0=";
        };
        vendorHash = "sha256-uQ1qKZLRwsgXKqSAERSqf+1cYKp6MTeVbfGs+qcdakE=";
        ldflags = [ "-s" "-w" ];
        subPackages = [ "cmd/goverter" ];
      };

      envPackages = attrValues {
        inherit (pkgs)
          coreutils tree
          git
          gcc pkg-config gnumake
          go gopls delve golangci-lint go-swagger
          hivemind
          python3
          openssl netcat
          postgresql sqlc goose
          mermerd
          buf protobuf grpcurl
          protoc-gen-go protoc-gen-go-grpc
          grpc-gateway
          protoc-gen-doc
        ;
        inherit
          goverter
        ;
      };
    in {
      packages.default = buildGoModule {
        name = "atlas";
        src = ./.;
        vendorHash = null;
      };
      devShells.default = mkShell {
        name = "atlas";
        packages = envPackages;
        shellHook = ''
          export GOTELEMETRY=off
          export GOPRIVATE=
          export GOSUMDB=off
        '';
      };
    });
}
