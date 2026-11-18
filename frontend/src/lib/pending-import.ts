// A file picked on the new-collection page, imported when the collection's records open.
// In memory only: a File cannot be stored, and a reload drops it (the import is then started by hand).
let pending: { collectionId: string; file: File } | null = null;

export const setPendingImport = (collectionId: string, file: File) => { pending = { collectionId, file }; };
export const pendingImport = (collectionId: string) => (pending?.collectionId === collectionId ? pending.file : null);
export const clearPendingImport = () => { pending = null; };
