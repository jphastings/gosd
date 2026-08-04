// A vendored, dependency-free streaming SHA-256, because WebCrypto's
// `crypto.subtle.digest` only hashes a value already fully in memory — it
// can't be fed one chunk at a time, which `substitute.ts` needs to hash the
// image and each placeholder incrementally as bytes flow through. No secret
// data is ever hashed here (image bytes and their manifest are both public),
// so timing side channels don't apply; correctness is pinned by NIST CAVP
// test vectors and by cross-checking random inputs against
// `crypto.subtle.digest` (see sha256.test.ts).
//
// Standard FIPS 180-4 SHA-256: 64-byte blocks, big-endian words, a 64-bit
// big-endian bit-length suffix. `lengthBytes` is tracked as a plain number
// (not BigInt) — safe well past any real image size, since 2**53 bytes is
// petabytes — and only split into the two 32-bit big-endian words the
// padding needs at `finalize()`.

const ROUND_CONSTANTS = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);

const INITIAL_HASH = new Uint32Array([
  0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
]);

const BLOCK_SIZE = 64;

function rotr(x: number, n: number): number {
  return (x >>> n) | (x << (32 - n));
}

export class Sha256 {
  private h: Uint32Array;
  private readonly pending: Uint8Array;
  private pendingLength: number;
  private lengthBytes: number;
  private finished: boolean;
  private readonly schedule: Uint32Array;

  constructor() {
    this.h = Uint32Array.from(INITIAL_HASH);
    this.pending = new Uint8Array(BLOCK_SIZE);
    this.pendingLength = 0;
    this.lengthBytes = 0;
    this.finished = false;
    this.schedule = new Uint32Array(64);
  }

  /** Feeds more bytes into the running hash. Throws if called after
   * `digest()` — finalization is destructive (padding is mixed into the
   * running state), so a finished instance can't resume. */
  update(data: Uint8Array): this {
    if (this.finished) {
      throw new Error(
        "Sha256.update() called after digest(); clone() before finalizing if you need both",
      );
    }
    this.lengthBytes += data.length;

    let offset = 0;
    if (this.pendingLength > 0) {
      const need = BLOCK_SIZE - this.pendingLength;
      const take = Math.min(need, data.length);
      this.pending.set(data.subarray(0, take), this.pendingLength);
      this.pendingLength += take;
      offset += take;
      if (this.pendingLength === BLOCK_SIZE) {
        this.processBlock(this.pending, 0);
        this.pendingLength = 0;
      }
    }

    while (offset + BLOCK_SIZE <= data.length) {
      this.processBlock(data, offset);
      offset += BLOCK_SIZE;
    }

    if (offset < data.length) {
      this.pendingLength = data.length - offset;
      this.pending.set(data.subarray(offset), 0);
    }

    return this;
  }

  /** Finalizes (idempotently) and returns the 32-byte digest. */
  digest(): Uint8Array {
    if (!this.finished) {
      this.finalize();
    }
    const out = new Uint8Array(32);
    const view = new DataView(out.buffer);
    for (let i = 0; i < 8; i++) {
      view.setUint32(i * 4, this.h[i], false);
    }
    return out;
  }

  digestHex(): string {
    return Array.from(this.digest())
      .map((b) => b.toString(16).padStart(2, "0"))
      .join("");
  }

  /** Returns an independent copy of the current running state — later
   * `update()` calls on either instance don't affect the other. Not used
   * for resuming yet (that's a follow-up), but keeps mid-stream checkpoints
   * possible without re-hashing from byte zero. */
  clone(): Sha256 {
    const copy = new Sha256();
    copy.h = Uint32Array.from(this.h);
    copy.pending.set(this.pending);
    copy.pendingLength = this.pendingLength;
    copy.lengthBytes = this.lengthBytes;
    copy.finished = this.finished;
    return copy;
  }

  private finalize(): void {
    const bitLength = this.lengthBytes * 8;
    const padded = new Uint8Array(this.pendingLength < 56 ? BLOCK_SIZE : BLOCK_SIZE * 2);
    padded.set(this.pending.subarray(0, this.pendingLength), 0);
    padded[this.pendingLength] = 0x80;

    const view = new DataView(padded.buffer);
    const highBits = Math.floor(bitLength / 0x100000000);
    const lowBits = bitLength >>> 0;
    view.setUint32(padded.length - 8, highBits, false);
    view.setUint32(padded.length - 4, lowBits, false);

    for (let offset = 0; offset < padded.length; offset += BLOCK_SIZE) {
      this.processBlock(padded, offset);
    }

    this.finished = true;
  }

  private processBlock(data: Uint8Array, offset: number): void {
    const w = this.schedule;
    const view = new DataView(data.buffer, data.byteOffset + offset, BLOCK_SIZE);
    for (let i = 0; i < 16; i++) {
      w[i] = view.getUint32(i * 4, false);
    }
    for (let i = 16; i < 64; i++) {
      const w15 = w[i - 15];
      const w2 = w[i - 2];
      const s0 = rotr(w15, 7) ^ rotr(w15, 18) ^ (w15 >>> 3);
      const s1 = rotr(w2, 17) ^ rotr(w2, 19) ^ (w2 >>> 10);
      w[i] = (w[i - 16] + s0 + w[i - 7] + s1) | 0;
    }

    let [a, b, c, d, e, f, g, h] = this.h;

    for (let i = 0; i < 64; i++) {
      const s1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25);
      const ch = (e & f) ^ (~e & g);
      const temp1 = (h + s1 + ch + ROUND_CONSTANTS[i] + w[i]) | 0;
      const s0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22);
      const maj = (a & b) ^ (a & c) ^ (b & c);
      const temp2 = (s0 + maj) | 0;

      h = g;
      g = f;
      f = e;
      e = (d + temp1) | 0;
      d = c;
      c = b;
      b = a;
      a = (temp1 + temp2) | 0;
    }

    this.h[0] = (this.h[0] + a) | 0;
    this.h[1] = (this.h[1] + b) | 0;
    this.h[2] = (this.h[2] + c) | 0;
    this.h[3] = (this.h[3] + d) | 0;
    this.h[4] = (this.h[4] + e) | 0;
    this.h[5] = (this.h[5] + f) | 0;
    this.h[6] = (this.h[6] + g) | 0;
    this.h[7] = (this.h[7] + h) | 0;
  }
}
