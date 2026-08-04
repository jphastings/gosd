import { webcrypto } from "node:crypto";
import { describe, expect, it } from "vitest";
import { Sha256 } from "./sha256.js";

function hex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

async function subtleDigestHex(bytes: Uint8Array): Promise<string> {
  const digest = await webcrypto.subtle.digest("SHA-256", bytes);
  return hex(new Uint8Array(digest));
}

describe("Sha256", () => {
  // FIPS 180-2 / NIST CAVP short and long message test vectors.
  it.each([
    ["", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"],
    ["abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"],
    [
      "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq",
      "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1",
    ],
  ])("matches the NIST vector for %j", (input, expected) => {
    expect(
      new Sha256().update(new TextEncoder().encode(input)).digestHex(),
    ).toBe(expected);
  });

  it("matches the NIST vector for one million repeated 'a'", () => {
    const input = new Uint8Array(1_000_000).fill(0x61);
    expect(new Sha256().update(input).digestHex()).toBe(
      "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0",
    );
  });

  it("cross-checks random-length random inputs against crypto.subtle.digest", async () => {
    for (const length of [
      0,
      1,
      55,
      56,
      63,
      64,
      65,
      127,
      128,
      1000,
      65536 + 37,
    ]) {
      const input = new Uint8Array(length);
      crypto.getRandomValues(input.subarray(0, Math.min(length, 65536)));
      for (let i = 65536; i < length; i++) input[i] = (i * 2654435761) % 256;

      const expected = await subtleDigestHex(input);
      expect(new Sha256().update(input).digestHex(), `length ${length}`).toBe(
        expected,
      );
    }
  });

  it("produces the same digest whether fed in one shot or in adversarial chunks", () => {
    const input = new Uint8Array(200_000);
    for (let i = 0; i < input.length; i++) input[i] = (i * 2654435761) % 256;
    const oneShot = new Sha256().update(input).digestHex();

    const chunkings: number[][] = [
      Array(input.length).fill(1),
      [1, 63, 64, 65, 1000, 55, 56, 57],
      [0, 200_000],
      [200_000],
    ];

    for (const sizes of chunkings) {
      const hasher = new Sha256();
      let offset = 0;
      let sizeIndex = 0;
      while (offset < input.length) {
        const size = sizes[sizeIndex % sizes.length] ?? input.length - offset;
        const end = Math.min(offset + size, input.length);
        hasher.update(input.subarray(offset, end));
        offset = end;
        sizeIndex++;
      }
      expect(hasher.digestHex()).toBe(oneShot);
    }
  });

  it("clone() produces an independent copy", () => {
    const base = new Sha256().update(new TextEncoder().encode("hello, "));
    const clone = base.clone();

    base.update(new TextEncoder().encode("base"));
    clone.update(new TextEncoder().encode("clone"));

    expect(base.digestHex()).not.toBe(clone.digestHex());
    expect(base.digestHex()).toBe(
      new Sha256().update(new TextEncoder().encode("hello, base")).digestHex(),
    );
    expect(clone.digestHex()).toBe(
      new Sha256().update(new TextEncoder().encode("hello, clone")).digestHex(),
    );
  });

  it("throws if update() is called after digest()", () => {
    const hasher = new Sha256().update(new TextEncoder().encode("x"));
    hasher.digest();
    expect(() => hasher.update(new TextEncoder().encode("y"))).toThrow(
      /after digest/,
    );
  });

  it("digest() is idempotent", () => {
    const hasher = new Sha256().update(new TextEncoder().encode("idempotent"));
    expect(hasher.digestHex()).toBe(hasher.digestHex());
  });
});
