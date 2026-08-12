import { describe, expect, it } from "vitest";
import { GosdInvalidEnvError } from "./errors.js";
import { renderEnvBody } from "./env.js";

describe("renderEnvBody", () => {
  it('writes one sorted KEY = "value" line per setting', () => {
    expect(renderEnvBody({ LOG_LEVEL: "debug", API_URL: "https://example.com" })).toBe(
      'API_URL = "https://example.com"\nLOG_LEVEL = "debug"\n',
    );
  });

  it("renders nothing for no settings, which clears the region to the padding", () => {
    expect(renderEnvBody({})).toBe("");
  });

  it("escapes what would otherwise end the string or the line", () => {
    const rendered = renderEnvBody({ TOKEN: 'a"b\\c\nd\tef' });
    expect(rendered).toBe('TOKEN = "a\\"b\\\\c\\nd\\tef"\n');
  });

  it("escapes a control character as \\uXXXX rather than passing it through", () => {
    expect(renderEnvBody({ TOKEN: "a\u0001b" })).toBe('TOKEN = "a\\u0001b"\n');
  });

  it("leaves non-ASCII text alone, since TOML basic strings are UTF-8", () => {
    expect(renderEnvBody({ GREETING: "héllo 👋" })).toBe('GREETING = "héllo 👋"\n');
  });

  it.each([
    ["1STARTS_WITH_A_DIGIT", "starts with a digit"],
    ["HAS-A-HYPHEN", "contains a hyphen"],
    ["HAS SPACE", "contains a space"],
    ["", "is empty"],
  ])("refuses %s, which %s and could never reach the app", (key) => {
    expect(() => renderEnvBody({ [key]: "x" })).toThrow(GosdInvalidEnvError);
  });

  it("refuses a GOSD_* key, which the device would ignore rather than apply", () => {
    expect(() => renderEnvBody({ GOSD_BOARD: "pi-zero-2w" })).toThrow(/GOSD_\*/);
  });

  it("refuses a non-string value rather than guessing how to format it", () => {
    expect(() => renderEnvBody({ PORT: 8080 as unknown as string })).toThrow(GosdInvalidEnvError);
  });
});
