import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { messages, type MessageKey } from "@/lib/i18n/messages";
import { errorMessage } from "./errors";

const t = (key: MessageKey) => messages["pt-BR"][key];

describe("errorMessage", () => {
  it("translates a known backend rule", () => {
    const err = new ApiError(400, { code: "INVALID_INPUT", message: "name must have 2 to 40 of a-z, 0-9, _ and -" });
    expect(errorMessage(err, "common.save_failed", t)).toBe(t("error.msg.role_name"));
  });

  it("matches a message that ends with a name", () => {
    const err = new ApiError(409, { code: "CONFLICT", message: "a default role cannot be deleted: user" });
    expect(errorMessage(err, "common.save_failed", t)).toBe(t("error.msg.default_role_deleted"));
  });

  it("keeps an unknown validation text and maps other codes", () => {
    expect(errorMessage(new ApiError(400, { code: "INVALID_INPUT", message: "invalid cursor" }), "common.save_failed", t)).toBe("invalid cursor");
    expect(errorMessage(new ApiError(403, { code: "FORBIDDEN", message: "forbidden" }), "common.save_failed", t)).toBe(t("error.forbidden"));
  });
});
