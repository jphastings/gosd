// A vite-plus-free mirror of vite.config.ts's test projects, for CI's
// node-20 runtime job ONLY. The package supports Node >= 20 (engines), but
// the vp toolchain does not: vite-plus 0.2.x's tsdown bundle calls
// Promise.withResolvers (Node 22+) despite its engines claim, so the
// node-20 CI leg runs vitest directly with this config — which must not
// import "vite-plus", or loading it would drag the same incompatible code
// into the Node 20 process. Keep the project split below in sync with
// vite.config.ts's `test` section.
import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    projects: [
      {
        test: {
          name: "unit",
          environment: "node",
          include: ["src/**/*.test.ts"],
        },
      },
      {
        test: {
          name: "integration",
          environment: "node",
          include: ["integration/**/*.test.ts"],
          testTimeout: 60000,
        },
      },
    ],
  },
});
