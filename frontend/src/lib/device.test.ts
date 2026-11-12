import { describe, expect, it } from "vitest";
import { describeDevice } from "./device";

describe("describeDevice", () => {
  it.each([
    ["Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.7977.167 Safari/537.36", "Chrome 152 · Linux", false],
    ["Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Safari/537.36 Edg/140.0", "Edge 140 · Windows", false],
    ["Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1", "Safari 18 · iOS", true],
    ["Mozilla/5.0 (Macintosh; Intel Mac OS X 14.0; rv:131.0) Gecko/20100101 Firefox/131.0", "Firefox 131 · macOS", false],
    ["curl/8.18.0", "curl 8.18.0", false],
  ])("%s", (ua, label, mobile) => {
    expect(describeDevice(ua)).toEqual({ label, mobile });
  });

  it("keeps an unknown agent readable", () => {
    expect(describeDevice("SomeBot")).toEqual({ label: "SomeBot", mobile: false });
    expect(describeDevice(null)).toEqual({ label: null, mobile: false });
  });
});
