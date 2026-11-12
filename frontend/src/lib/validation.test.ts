import { describe, expect, it } from "vitest";
import { email, newPassword, personName, price, ready, roleName, sameAs } from "./validation";

describe("validation rules", () => {
  it("say nothing about an empty value", () => {
    for (const rule of [personName, email, price, roleName, newPassword, sameAs("x")]) expect(rule("")).toBeNull();
  });

  it("follow the backend limits", () => {
    expect(personName("A")).toBe("error.msg.person_name");
    expect(personName(" Ana ")).toBeNull();
    expect(email("ana@acme")).toBe("error.msg.email");
    expect(email("ana@acme.io")).toBeNull();
    expect(price("12,5")).toBeNull();
    expect(price("12.345")).toBe("error.msg.price_format");
    expect(price("-5")).toBe("error.msg.price_negative");
    expect(roleName("Gestores")).toBe("error.msg.role_name");
    expect(roleName("gestores")).toBeNull();
    expect(newPassword("short")).toBe("error.msg.password_short");
    expect(newPassword("é".repeat(65))).toBe("error.msg.password_long"); // 130 bytes, as the server counts
    expect(sameAs("a")("b")).toBe("auth.password_mismatch");
  });

  it("ready needs every field filled and valid", () => {
    expect(ready(["Ana", personName], ["ana@acme.io", email])).toBe(true);
    expect(ready(["Ana", personName], ["", email])).toBe(false);
    expect(ready(["A", personName])).toBe(false);
  });
});
