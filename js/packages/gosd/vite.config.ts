import { defineConfig } from "vite-plus";

// vp pack builds both entries below concurrently, so neither may use tsdown's
// own `clean` (a race: whichever finishes last would wipe what the other just
// wrote). `dist` is cleared once, up front, by the "build" script instead
// (`rm -rf dist && vp pack`).
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
  pack: [
    // The public library entry: ESM with a bundled .d.ts, resolved by the
    // "./downloads" export condition in package.json (dist path is part of
    // that FROZEN contract). dts generator is pinned to "oxc" (isolated
    // declarations) because the default/"tsgo" generator, auto-selected
    // whenever TypeScript 7's native-preview compiler is installed (see the
    // root devDependency), either crashes or silently emits a bogus,
    // non-declaration ".ts" file instead of ".d.ts" — a rolldown-plugin-dts
    // incompatibility with TS 7's still-experimental API, not something
    // fixable from this config alone.
    {
      entry: { "downloads/index": "src/downloads/index.ts" },
      format: "esm",
      platform: "neutral",
      dts: { generator: "oxc" },
      minify: false,
      clean: false,
    },
    // The service-worker entry: a classic script the integrator hosts
    // themselves (see src/sw/gosd-download-sw.ts's header comment) — no
    // import/export statements, so iife's wrapper is harmless and its
    // absence of a module graph is exactly what's needed. No .d.ts: this
    // file is never imported, only hosted as a static asset. The explicit
    // entryFileNames avoids tsdown's default ".iife" filename infix (added
    // for any non-esm format) — the dist path is part of the FROZEN contract.
    {
      entry: { "sw/gosd-download-sw": "src/sw/gosd-download-sw.ts" },
      format: "iife",
      platform: "browser",
      dts: false,
      minify: false,
      clean: false,
      outputOptions: { entryFileNames: "sw/gosd-download-sw.js" },
    },
  ],
});
