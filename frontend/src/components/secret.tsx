"use client";

import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n/provider";
import { copyText } from "@/lib/utils";

export function CopyButton({ text }: { text: string }) {
  const { t } = useI18n();
  const [done, setDone] = useState(false);
  return (
    <Button type="button" size="sm" variant="outline" onClick={() => void copyText(text).then(() => { setDone(true); setTimeout(() => setDone(false), 1500); })}>
      {done ? <Check className="size-4" /> : <Copy className="size-4" />}{t(done ? "integrations.copied" : "integrations.copy")}
    </Button>
  );
}

/** A secret shown once, right after it was made: the server keeps only its hash (keys) or sealed (webhooks). */
export function Secret({ value, onClose }: { value: string; onClose: () => void }) {
  const { t } = useI18n();
  return (
    <div role="status" className="space-y-2 rounded-md border border-warn/40 bg-warn/10 p-3 text-sm">
      <p className="font-medium">{t("integrations.secret_once")}</p>
      <code className="block break-all rounded bg-background px-2 py-1.5 font-mono text-xs">{value}</code>
      <div className="flex gap-2">
        <CopyButton text={value} />
        <Button type="button" size="sm" variant="ghost" onClick={onClose}>{t("integrations.secret_saved")}</Button>
      </div>
    </div>
  );
}
