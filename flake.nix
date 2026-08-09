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

          # This derivation depends on Go twice over, and only one of the
          # two is negotiable.
          #
          # Build time (compiling gosd itself, here): buildGoModule sets
          # GOTOOLCHAIN=local and a nix sandbox has no user PATH, so
          # pkgs.go MUST already satisfy go.mod's `go` directive - there's
          # no toolchain fetch to fall back on. That directive can't be
          # relaxed to a bare major.minor either: `go mod tidy` raises it
          # to the maximum of every dependency's own floor (tailscale.com
          # sets today's). So when a dependency bump raises it, `nix flake
          # update` belongs in the SAME PR - and if nixos-unstable hasn't
          # shipped that Go patch release yet, this build stays red until
          # it does. That's a wait, not a broken change; it cost PR #231
          # once already, with "go.mod requires go >= 1.26.5 (running go
          # 1.26.4; GOTOOLCHAIN=local)".
          #
          # Run time (gosd compiling the user's app): see postInstall.

          # Must match go.mod/go.sum. When they change, CI's "nix build"
          # job (.github/workflows/ci.yml) fails with
          #   hash mismatch in fixed-output derivation ...
          #     got: sha256-...
          # Paste that "got:" value here.
          vendorHash = "sha256-dskNeCcI7gzc1z3v7DMgfmQPHwiNzyOBFYtDT6H0ew8=";

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
          #     gosd-init). Appended with --suffix rather than --prefix
          #     on purpose: gosd's job is compiling the *user's* app, so
          #     a user- or CI-provided go must keep winning - someone who
          #     installed a newer Go to reach newer language features
          #     should get it. This bundle is the fallback that makes
          #     `nix run github:jphastings/gosd -- build ./cmd/myapp`
          #     work on a machine with no Go at all, which is what README
          #     promises. A user go that turns out to be too old is made
          #     legible by internal/build's explainBuildFailure, rather
          #     than overruled here.
          #  2. gosd-init's source. A nix-built gosd can't locate it by
          #     itself: the binary carries no module version
          #     (Main.Version is "(devel)", so the module-cache rung of
          #     internal/build/gosdinit.go's ladder fails), and the
          #     dev-checkout rung fails too because runtime.Caller
          #     resolves to this sandbox's build directory, long gone by
          #     the time a user runs gosd. (Not -trimpath, despite
          #     appearances: buildGoModule adds that only when
          #     allowGoReference is unset, and it's true above.) Ship the
          #     source this very
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
