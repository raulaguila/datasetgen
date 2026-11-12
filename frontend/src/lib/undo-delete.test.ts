import { afterEach, expect, it, vi } from "vitest";

const shown: { action: { onClick: () => void }; onAutoClose: () => void; onDismiss: () => void }[] = [];
vi.mock("sonner", () => ({ toast: (_: string, o: (typeof shown)[number]) => shown.push(o) }));
const { deleteWithUndo } = await import("./undo-delete");

afterEach(() => void shown.splice(0));

function start() {
  const run = vi.fn(async () => {});
  const onSettled = vi.fn();
  deleteWithUndo({ message: "", undoLabel: "", run, onError: () => {}, onSettled });
  return { run, onSettled, toast: shown[0] };
}

it("deletes once when the toast closes, however it closes", async () => {
  const { run, onSettled, toast } = start();
  toast.onAutoClose();
  toast.onDismiss();
  await vi.waitFor(() => expect(onSettled).toHaveBeenCalledOnce());
  expect(run).toHaveBeenCalledOnce();
});

it("undo keeps the rows even when the toast closes afterwards", () => {
  const { run, onSettled, toast } = start();
  toast.action.onClick();
  toast.onDismiss();
  expect(run).not.toHaveBeenCalled();
  expect(onSettled).toHaveBeenCalledOnce();
});
