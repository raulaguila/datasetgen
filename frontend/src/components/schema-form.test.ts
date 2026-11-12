import { describe, expect, it } from "vitest";
import { forDraft } from "./schema-form";

const props = { properties: { a: { type: "string", format: "date" } }, required: ["a"], type: "object" };

describe("forDraft", () => {
  it("validates schemas that declare draft 2020-12", () => {
    const { schema, validator } = forDraft({ $schema: "https://json-schema.org/draft/2020-12/schema", ...props });
    expect(schema).not.toHaveProperty("$schema");
    expect(validator.isValid(schema, { a: "2020-01-01" }, schema)).toBe(true);
    expect(validator.isValid(schema, {}, schema)).toBe(false);
    expect(validator.isValid(schema, { a: "ontem" }, schema)).toBe(false);
  });

  it("keeps the draft-07 validator for draft-07 schemas", () => {
    const { schema, validator } = forDraft({ $schema: "http://json-schema.org/draft-07/schema#", ...props, items: [{ type: "string" }] });
    expect(validator.isValid(schema, { a: "2020-01-01" }, schema)).toBe(true);
  });
});
