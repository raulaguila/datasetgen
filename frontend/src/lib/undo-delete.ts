import { toast } from "sonner";

const UNDO_MS = 5_000;

/**
 * Deletes after a short window the user can undo. The caller hides the rows at
 * once; onSettled runs after the delete, whether it worked or not, and after an
 * undo. Closing the tab inside the window keeps the rows.
 */
export function deleteWithUndo(opts: {
  message: string;
  undoLabel: string;
  run: () => Promise<void>;
  onError: (err: unknown) => void;
  onSettled: () => void;
}) {
  let settled = false;
  const commit = () => {
    if (settled) return;
    settled = true;
    opts.run().catch(opts.onError).finally(opts.onSettled);
  };
  toast(opts.message, {
    duration: UNDO_MS,
    action: {
      label: opts.undoLabel,
      onClick: () => {
        settled = true;
        opts.onSettled();
      },
    },
    onAutoClose: commit,
    onDismiss: commit,
  });
}
