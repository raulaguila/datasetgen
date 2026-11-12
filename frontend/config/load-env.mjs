import { existsSync, readFileSync } from "node:fs";
import { parseEnv } from "node:util";

// Node's dotenv parser, so quotes and comments follow standard .env rules.
// Container/process configuration takes priority.
export function loadConfigDotEnv(envPath, target = process.env) {
  if (!existsSync(envPath)) return;
  for (const [key, value] of Object.entries(parseEnv(readFileSync(envPath, "utf8")))) {
    if (target[key] === undefined) target[key] = value;
  }
}
