import { Validator, type OutputUnit, type Schema, type SchemaDraft } from "@cfworker/json-schema";
import { toErrorSchema, type RJSFSchema, type RJSFValidationError, type ValidationData, type ValidatorType } from "@rjsf/utils";

/**
 * An rjsf validator built on @cfworker/json-schema, which interprets schemas.
 * ajv (rjsf's default) compiles them with new Function, which the production
 * Content-Security-Policy blocks (no 'unsafe-eval'), so validation silently failed.
 */

// Keywords that only say "something inside failed"; the inner unit names what.
const WRAPPERS = new Set(["properties", "items", "prefixItems", "additionalItems", "patternProperties", "allOf", "$ref", "$dynamicRef", "contains", "dependentSchemas", "if", "then", "else"]);

/** A message for one failed keyword in the app language, or undefined to keep the library's English text. */
export type Translate = (keyword: string) => string | undefined;

/** The draft the server will use: 2020-12 unless $schema names draft-07 or older. */
export function draftOf(schema: object): SchemaDraft {
  const declared = (schema as { $schema?: unknown }).$schema;
  if (typeof declared !== "string" || declared.includes("2020-12")) return "2020-12";
  if (declared.includes("2019-09")) return "2019-09";
  return declared.includes("draft-04") ? "4" : "7";
}

/** "#/parametros/0" → ["parametros", "0"] */
function segments(location: string): string[] {
  return location.replace(/^#\/?/, "").split("/").filter(Boolean).map((p) => decodeURIComponent(p).replace(/~1/g, "/").replace(/~0/g, "~"));
}

export function toRjsfErrors(units: OutputUnit[], translate: Translate): RJSFValidationError[] {
  const out: RJSFValidationError[] = [];
  for (const unit of units) {
    if (WRAPPERS.has(unit.keyword)) continue;
    const path = segments(unit.instanceLocation);
    // "required" fails on the parent object; show it under the missing field instead.
    const missing = unit.keyword === "required" ? /property "(.+)"/.exec(unit.error)?.[1] : undefined;
    if (missing !== undefined) path.push(missing);
    const property = path.length ? "." + path.join(".") : ".";
    const message = translate(unit.keyword) ?? unit.error;
    out.push({ name: unit.keyword, property, message, stack: `${property} ${message}`, schemaPath: unit.keywordLocation });
  }
  return out;
}

export function schemaValidator(translate: Translate): ValidatorType {
  // Building a Validator walks the whole schema; live validation runs on every
  // key press, so keep one per (root, sub-schema) pair while those objects live.
  const cache = new WeakMap<object, WeakMap<object, Validator>>();
  const run = (schema: RJSFSchema, data: unknown, root: RJSFSchema = schema) => {
    let byRoot = cache.get(root);
    if (!byRoot) cache.set(root, (byRoot = new WeakMap()));
    let validator = byRoot.get(schema);
    if (!validator) {
      // Sub-schemas rjsf checks may $ref the root's definitions; carry them along.
      const { definitions, $defs } = root as { definitions?: unknown; $defs?: unknown };
      const full = { ...(definitions ? { definitions } : {}), ...($defs ? { $defs } : {}), ...(schema as object) } as Schema;
      byRoot.set(schema, (validator = new Validator(full, draftOf(root), false)));
    }
    return validator.validate(data);
  };
  return {
    validateFormData(formData, schema): ValidationData<unknown> {
      const errors = toRjsfErrors(run(schema, formData).errors, translate);
      return { errors, errorSchema: toErrorSchema(errors) };
    },
    isValid(schema, formData, rootSchema) {
      try {
        return run(schema, formData, rootSchema).valid;
      } catch {
        return false; // an invalid schema validates nothing, as the interface asks
      }
    },
    rawValidation(schema, formData) {
      try {
        return { errors: run(schema, formData).errors as never[] };
      } catch (err) {
        return { validationError: err as Error };
      }
    },
  };
}
