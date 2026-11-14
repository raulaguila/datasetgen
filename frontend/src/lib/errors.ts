import { ApiError } from "@/lib/api/client";
import type { MessageKey } from "@/lib/i18n/messages";

const BY_CODE: Record<string, MessageKey> = {
  INVALID_CREDENTIALS: "error.invalid_credentials",
  TOO_MANY_REQUESTS: "error.too_many_requests",
  TOKEN_EXPIRED: "error.token_expired",
  FORBIDDEN: "error.forbidden",
  NOT_FOUND: "error.not_found",
  ALREADY_EXISTS: "error.already_exists",
  CONFLICT: "error.conflict",
  RESOURCE_IN_USE: "error.resource_in_use",
  UNAVAILABLE: "error.unavailable",
};

// The backend answers in English; these are the rules a form can break. Each
// entry is the start of the backend message, so a trailing name still matches.
const BY_MESSAGE: [string, MessageKey][] = [
  ["name must have 2 to 40 of a-z", "error.msg.role_name"],
  ["name must be 2-80 characters", "error.msg.person_name"],
  ["invalid email", "error.msg.email"],
  ["password too short", "error.msg.password_short"],
  ["password too long", "error.msg.password_long"],
  ["password too common", "error.msg.password_common"],
  ["password cannot be your name or email", "error.msg.password_personal"],
  ["current password is wrong", "error.msg.current_password"],
  ["price required", "error.msg.price_required"],
  ["price must be a number", "error.msg.price_number"],
  ["price must be a decimal", "error.msg.price_format"],
  ["from must be before to", "error.msg.date_range"],
  ["more than 10000 records", "error.msg.export_too_big"],
  ["a default role cannot be disabled", "error.msg.default_role_disabled"],
  ["a default role cannot be deleted", "error.msg.default_role_deleted"],
  ["the account is already activated", "error.msg.already_activated"],
  ["your own account cannot be disabled", "error.msg.own_account"],
  ["end your own sessions from your profile", "error.msg.own_sessions"],
  ["the record was changed by someone else", "error.msg.stale"],
  ["records repeat values of a field marked unique", "error.msg.unique_repeated"],
  ["a unique field cannot be filled in bulk", "error.msg.unique_bulk"],
  ["picture must be at most", "error.msg.avatar_size"],
  ["picture must be PNG", "error.msg.avatar_type"],
];

/**
 * A message the user can act on, in the app's language. A known backend rule
 * gets its translation; other validation errors keep the backend text; anything
 * else falls back to a line per error code.
 */
export function errorMessage(err: unknown, fallback: MessageKey, t: (key: MessageKey) => string): string {
  if (!(err instanceof ApiError)) return t("error.network");
  const known = BY_MESSAGE.find(([start]) => err.message.startsWith(start));
  if (known) return t(known[1]);
  if (err.code === "INVALID_INPUT" || err.code === "VALIDATION_FAILED") return err.message;
  if (err.code === "TOO_MANY_REQUESTS" && err.retryAfter) {
    return t("error.too_many_requests_wait").replace("{n}", String(Math.max(1, Math.ceil(err.retryAfter / 60))));
  }
  const key = BY_CODE[err.code];
  return key ? t(key) : t(fallback);
}
