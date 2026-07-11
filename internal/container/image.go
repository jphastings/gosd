package container

// KernelBuildImage is the base image gosd build-kernel runs cross-compiles
// inside. It's pinned by digest rather than a floating tag (docker.io/
// library/debian:bookworm would move under us) so a kernel build today
// reproduces byte-for-byte on a machine that runs it a year from now;
// bumping the digest is a deliberate, reviewed change, not something that
// should happen incidentally in an unrelated commit.
//
// Digest obtained 2026-07-11 two ways, which agreed:
//  1. `docker buildx imagetools inspect docker.io/library/debian:bookworm`
//     (its top-level "Digest:" is the multi-arch image-index digest).
//  2. Docker Hub API: `curl -s
//     https://hub.docker.com/v2/repositories/library/debian/tags/bookworm`,
//     the top-level "digest" field of the JSON response.
const KernelBuildImage = "docker.io/library/debian:bookworm@sha256:30482e873082e906a4908c10529180aefb6f77620aea7404b909829fadc5d168"
