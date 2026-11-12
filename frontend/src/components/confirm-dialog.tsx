"use client";

import { useRef, useState } from "react";
import { errorMessage } from "@/lib/errors";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n/provider";

/** Explicit, translated feedback for partial batches; API errors stay sanitized. */
export class ConfirmationError extends Error {}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel,
  destructive,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  destructive?: boolean;
  onConfirm: () => void | Promise<void>;
}) {
  const { t } = useI18n();
  const inFlight = useRef(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cancel = cancelLabel ?? t("common.cancel");
  const confirm = confirmLabel ?? t("common.confirm");
  const changeOpen = (next: boolean) => {
    if (inFlight.current) return;
    setError(null);
    onOpenChange(next);
  };
  const submit = async () => {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    try {
      await onConfirm();
      onOpenChange(false);
    } catch (e) {
      setError(e instanceof ConfirmationError ? e.message : errorMessage(e, "common.confirm_failed", t));
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  };
  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="sm:max-w-sm" aria-busy={pending} showCloseButton={!pending}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter>
          <Button variant="ghost" disabled={pending} onClick={() => changeOpen(false)}>
            {cancel}
          </Button>
          <Button
            variant={destructive ? "destructive" : "default"}
            disabled={pending}
            onClick={() => void submit()}
          >
            {pending ? t("common.confirming") : confirm}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
