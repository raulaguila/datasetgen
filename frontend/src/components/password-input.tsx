"use client";

import * as React from "react";
import { Eye, EyeOff } from "lucide-react";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n/provider";

// Password field with a show/hide toggle and optional Caps Lock callback.
// `type`/`className` are applied after the forwarded props so a caller can't
// override the toggle; onKeyUp is chained so capslock + caller handlers both run.
export function PasswordInput({
  className,
  onCapsLock,
  onKeyUp,
  ...props
}: React.ComponentProps<typeof Input> & { onCapsLock?: (on: boolean) => void }) {
  const { t } = useI18n();
  const [show, setShow] = React.useState(false);
  return (
    <div className="relative min-w-0">
      <Input
        {...props}
        type={show ? "text" : "password"}
        className={cn("pr-12", className)}
        onKeyUp={(e) => {
          onKeyUp?.(e);
          onCapsLock?.(e.getModifierState?.("CapsLock") ?? false);
        }}
      />
      <button
        type="button"
        aria-pressed={show}
        aria-label={show ? t("auth.hide_password") : t("auth.show_password")}
        onClick={() => setShow((s) => !s)}
        className="absolute right-0 top-1/2 flex size-[var(--control-h)] -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:text-foreground"
      >
        {show ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
      </button>
    </div>
  );
}
