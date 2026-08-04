import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { GosdSaveFailedError } from "../errors.js";
import { PROTOCOL, createPumpWritable } from "./service-worker.js";

// service-worker.ts (src/downloads/sinks/) -> package root.
const packageRoot = fileURLToPath(new URL("../../..", import.meta.url));

describe("service-worker handshake protocol", () => {
  it("PROTOCOL matches the literal in ../../sw/gosd-download-sw.ts", () => {
    const workerSource = readFileSync(
      path.join(packageRoot, "src/sw/gosd-download-sw.ts"),
      "utf8",
    );
    const match = workerSource.match(/const PROTOCOL = (\d+);/);
    expect(
      match,
      "gosd-download-sw.ts should declare `const PROTOCOL = <n>;`",
    ).not.toBeNull();
    expect(Number(match![1])).toBe(PROTOCOL);
  });
});

describe("gosd-download-sw.ts build output", () => {
  it("contains no import/export tokens once compiled", () => {
    const distPath = path.join(packageRoot, "dist/sw/gosd-download-sw.js");
    let compiled: string;

    if (existsSync(distPath)) {
      compiled = readFileSync(distPath, "utf8");
    } else {
      // Self-contained fallback so this test is deterministic even before
      // `npm run build` has run: compile it fresh, via the same
      // tsconfig.sw.json program, into a scratch directory.
      const tmpDir = mkdtempSync(path.join(os.tmpdir(), "gosd-sw-build-"));
      const workspaceRoot = path.join(packageRoot, "..", "..");
      const tscBin = path.join(
        workspaceRoot,
        "node_modules",
        ".bin",
        process.platform === "win32" ? "tsc.cmd" : "tsc",
      );
      execFileSync(tscBin, ["-p", "tsconfig.sw.json", "--outDir", tmpDir], {
        cwd: packageRoot,
        stdio: "pipe",
      });
      compiled = readFileSync(path.join(tmpDir, "gosd-download-sw.js"), "utf8");
    }

    expect(compiled).not.toMatch(/\bimport\b/);
    expect(compiled).not.toMatch(/\bexport\b/);
    expect(compiled.length).toBeGreaterThan(0);
  });
});

describe("createPumpWritable (Safari fallback backpressure)", () => {
  function pumpPair(ackTimeoutMs?: number) {
    const channel = new MessageChannel();
    const writable = createPumpWritable(
      channel.port1,
      "fixture.img",
      ackTimeoutMs,
    );
    return { channel, writer: writable.getWriter() };
  }

  it("holds each write unresolved until the worker acks it", async () => {
    const { channel, writer } = pumpPair();
    const received: string[] = [];
    let ack: () => void = () => {};
    channel.port2.onmessage = (event: MessageEvent) => {
      const data = event.data as { type?: string; chunk?: Uint8Array };
      if (data.type === "chunk") {
        received.push(new TextDecoder().decode(data.chunk));
        ack = () => channel.port2.postMessage({ type: "ack" });
      }
    };

    let settled = false;
    const write = writer.write(new TextEncoder().encode("one")).then(() => {
      settled = true;
    });
    // Give the message a chance to arrive; the write must still be pending.
    await new Promise((r) => setTimeout(r, 20));
    expect(received).toEqual(["one"]);
    expect(settled).toBe(false);

    ack();
    await write;
    expect(settled).toBe(true);

    channel.port1.close();
    channel.port2.close();
  });

  it("fails the write when no ack arrives within the timeout", async () => {
    const { channel, writer } = pumpPair(50);
    channel.port2.onmessage = () => {}; // a worker that never acks
    await expect(writer.write(new Uint8Array([1]))).rejects.toThrow(
      GosdSaveFailedError,
    );
    channel.port1.close();
    channel.port2.close();
  });

  it("fails the write when the worker reports a write error", async () => {
    const { channel, writer } = pumpPair();
    channel.port2.onmessage = (event: MessageEvent) => {
      const data = event.data as { type?: string };
      if (data.type === "chunk") {
        channel.port2.postMessage({ type: "error", reason: "disk full" });
      }
    };
    await expect(writer.write(new Uint8Array([1]))).rejects.toThrow(
      /disk full/,
    );
    channel.port1.close();
    channel.port2.close();
  });
});
