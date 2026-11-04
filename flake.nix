{
  description = "Z-Library CLI and terminal interface";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "aarch64-darwin" "aarch64-linux" "x86_64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in
        {
          default = self.packages.${system}.zlib-cli;
          zlib-cli = pkgs.buildGoModule {
            pname = "zlib-cli";
            version = "0-unstable-${builtins.substring 0 8 (self.lastModifiedDate or "19700101")}";
            src = self;
            vendorHash = "sha256-qXzhQI8LCMnVi5Mu1a8QHPhSyzHeqxQEN0wfb0tlTms=";
            subPackages = [ "cmd/zlib" ];
            env.CGO_ENABLED = "0";
            ldflags = [
              "-s"
              "-w"
              "-X=github.com/heartleo/zlib/internal/zlib.Commit=${self.rev or "dirty"}"
            ];

            meta = {
              description = "Z-Library CLI and terminal interface";
              license = pkgs.lib.licenses.mit;
              mainProgram = "zlib";
              platforms = systems;
            };
          };
        });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          meta.description = "Z-Library CLI and terminal interface";
          program = "${self.packages.${system}.zlib-cli}/bin/zlib";
        };
      });

      checks = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          package = self.packages.${system}.zlib-cli;
        in
        {
          build = package;
          cli = pkgs.runCommand "zlib-cli-smoke-test" { } ''
            ${package}/bin/zlib --help > "$out"
            ${package}/bin/zlib version >> "$out"
          '';
        });
    };
}
