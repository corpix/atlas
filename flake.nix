{
  inputs = {
    nixpkgs.url = "tarball+https://git.tatikoma.dev/corpix/nixpkgs/archive/corpix.tar.gz";
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

      envPackages = attrValues {
        inherit (pkgs)
          coreutils tree
          git
          gcc pkg-config gnumake just
          go gopls delve golangci-lint go-swagger
          betteralign goverter
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
          export GOPRIVATE=git.tatikoma.dev/
          export GOSUMDB=off
          export GOPROXY=https://goproxy.tatikoma.dev

          export NIX_PATH=nixpkgs=${nixpkgs}
        '';
      };
    });
}
