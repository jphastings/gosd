{
  description = "GoSD - turn a Go main package into flashable SD-card images for small ARM boards";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      # The gosd CLI's supported hosts (see CLAUDE.md): macOS and Linux,
      # amd64/arm64.
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        default = gosd;
        gosd = pkgs.buildGoModule {
          pname = "gosd";
          version = self.shortRev or self.dirtyShortRev or "unknown";
          src = self;

          # Must match go.mod/go.sum. When they change, CI's "nix build"
          # job (.github/workflows/ci.yml) fails with
          #   hash mismatch in fixed-output derivation ...
          #     got: sha256-...
          # Paste that "got:" value here.
          vendorHash = "sha256-1pPplh3xJcqyuQesLgmh/Hv6tnaIBoWUqio5CVMF5cM=";

          subPackages = [ "cmd/gosd" ];
          env.CGO_ENABLED = "0";

          # The full test suite (including image-assembly integration tests)
          # runs in regular CI on every PR; re-running it inside the nix
          # sandbox adds minutes and no coverage.
          doCheck = false;

          # gosd invokes the go toolchain at run time (to cross-compile the
          # user's app and gosd-init), so referencing go from the output is
          # the point, not an accident.
          allowGoReference = true;

          nativeBuildInputs = [ pkgs.makeWrapper ];

          # gosd needs two things beyond its own binary at run time:
          #
          #  1. A Go toolchain, to cross-compile the user's app (and
          #     gosd-init) - appended to PATH as a fallback so a user- or
          #     CI-provided go still wins.
          #  2. gosd-init's source. A nix-built gosd can't locate it by
          #     itself: the binary carries no module version
          #     (Main.Version is "(devel)", so the module-cache rung of
          #     internal/build/gosdinit.go's ladder fails) and -trimpath
          #     erases the compiled-from checkout path (so the
          #     dev-checkout rung fails too). Ship the source this very
          #     package was built from - vendor directory included, so
          #     building gosd-init needs no network at all - and point
          #     the GOSD_INIT_SRC hook at it (--gosd-init-src still
          #     overrides).
          postInstall = ''
            mkdir -p $out/share
            cp -r . $out/share/gosd-src
            wrapProgram $out/bin/gosd \
              --set-default GOSD_INIT_SRC $out/share/gosd-src/cmd/gosd-init \
              --suffix PATH : ${pkgs.lib.makeBinPath [ pkgs.go ]}
          '';

          meta = {
            description = "Turn a Go main package into flashable SD-card images for small ARM boards";
            homepage = "https://github.com/jphastings/gosd";
            license = pkgs.lib.licenses.mit;
            mainProgram = "gosd";
          };
        };
      });
    };
}
