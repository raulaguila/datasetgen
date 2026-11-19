"use client";

import { useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { FilePlus2, FileUp, Sheet } from "lucide-react";
import { toast } from "sonner";
import { FieldNote, FormSection, FormSections, SaveBar } from "@/components/bits";
import { CollectionAvatar } from "@/components/collection-avatar";
import { IconPicker } from "@/components/icon-picker";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api/client";
import { collections } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { COLLECTION_ICONS } from "@/lib/collection-icons";
import { TEMPLATE_IDS, templateFields, type TemplateId } from "@/lib/collection-templates";
import { errorMessage } from "@/lib/errors";
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import { Cancelled, inferSchema } from "@/lib/import-client";
import { collectionHead } from "@/lib/import-stream";
import { templateSchema } from "@/lib/infer-schema";
import { setPendingImport } from "@/lib/pending-import";
import { cn } from "@/lib/utils";
import { collectionName, description as descriptionRule, ready } from "@/lib/validation";
import { starter } from "./_parts";

type Start = "scratch" | "file";
const STARTS: { id: Start; icon: typeof FilePlus2 }[] = [
  { id: "scratch", icon: FilePlus2 },
  { id: "file", icon: Sheet },
];
const DATA_FILES = ".json,.jsonl,.ndjson,.csv,.xlsx,application/json,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";

/**
 * A new collection: its name, then where its fields come from, a template or a
 * file. A whole-collection export brings its name, icon, fields and records; a
 * data file brings fields from its columns and its rows. Fields are edited
 * afterwards on the collection's Fields page.
 */
export function NewCollectionForm() {
  const { t, locale } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [icon, setIcon] = useState("");
  const [tried, setTried] = useState(false);
  const [start, setStart] = useState<Start>("scratch");
  const [template, setTemplate] = useState<TemplateId>("basic");
  const [file, setFile] = useState<{ file: File; schema: Record<string, unknown> } | null>(null);
  const [fileNote, setFileNote] = useState<string | null>(null);
  const [importRows, setImportRows] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const nameError = collectionName(name) ?? (tried && !name.trim() ? "collections.name_needed" : null);

  const templateFor = (id: TemplateId) => (id === "basic" ? starter(t) : templateSchema(templateFields(locale, id)));
  // What Create sends; null while the file is still missing.
  const schema = start === "scratch" ? templateFor(template) : file?.schema ?? null;
  const missing = !schema;
  const dirty = !!name.trim() || !!desc.trim() || !!file;
  useUnsavedGuard(dirty, t("roles.leave_unsaved"));

  // Read in the import worker: a large file keeps the page answering, with its progress shown.
  const reading = useRef<AbortController | null>(null);
  const pickFile = async (f: File) => {
    reading.current?.abort();
    const ctl = (reading.current = new AbortController());
    setFile(null);
    const shown = (read: number) => setFileNote(t("collections.template_file_reading").replace("{file}", f.name).replace("{pct}", String(f.size ? Math.min(100, Math.round((read / f.size) * 100)) : 0)));
    shown(0);
    try {
      // A whole-collection export says everything on its first line; its blanks fill the details left empty here.
      const head = await collectionHead(f);
      if (ctl.signal.aborted) return;
      if (head) {
        setFile({ file: f, schema: head.schema });
        if (!name.trim()) setName(head.name);
        if (!desc.trim()) setDesc(head.description);
        if (!icon && Object.hasOwn(COLLECTION_ICONS, head.icon)) setIcon(head.icon);
        return setFileNote(t("collections.start.collection_loaded").replace("{file}", f.name));
      }
      const next = await inferSchema(f, shown, ctl.signal);
      if (!next) return setFileNote(t("collections.template_file_empty"));
      setFile({ file: f, schema: next });
      setFileNote(t("collections.start.file_loaded").replace("{n}", String(Object.keys(next.properties as object).length)).replace("{file}", f.name));
    } catch (err) {
      if (err instanceof Cancelled) return;
      setFileNote(err instanceof Error && err.message.startsWith("xlsx:") ? t("import.bad_xlsx") : t("collections.template_file_empty"));
    }
  };

  const create = useMutation<Collection>({
    mutationFn: () => collections.create({ name: name.trim(), description: desc.trim(), icon, schema: schema! }),
    onSuccess: (saved) => {
      client.setQueryData(["collections", "detail", saved.id], saved);
      void client.invalidateQueries({ queryKey: ["collections"] });
      toast.success(t("collections.created"));
      if (start === "file" && importRows && file) setPendingImport(saved.id, file.file);
      router.replace(start === "scratch" ? `/collections/${saved.id}/fields` : importRows ? `/collections/${saved.id}` : `/collections/${saved.id}/overview`);
    },
    // The backend names the broken schema rule; it reads better than a generic failure.
    onError: (err) => setError(err instanceof ApiError && err.field === "schema" ? err.message : errorMessage(err, "common.save_failed", t)),
  });

  const fieldTitles = (s: Record<string, unknown> | null) =>
    Object.entries((s?.properties ?? {}) as Record<string, { title?: string }>).map(([k, p]) => p.title || k);

  return (
    <form noValidate onSubmit={(e) => {
      e.preventDefault();
      if (!ready([name, collectionName])) {
        setTried(true);
        document.getElementById("c-name")?.focus();
        return;
      }
      if (!missing && !descriptionRule(desc)) create.mutate();
    }}
      // Ctrl/⌘+Enter creates from any field.
      onKeyDown={(e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); e.currentTarget.requestSubmit(); } }}>
      <FormSections>
        <FormSection title={t("collections.details")} description={t("collections.details_desc")} wide headAbove>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="c-name">{t("auth.name")}</Label>
              <div className="flex items-center gap-2">
                <CollectionAvatar collection={{ id: "new", name: name.trim() || t("collections.new"), icon }} className="size-9 shrink-0 rounded-md text-sm font-semibold" />
                <Input id="c-name" required autoFocus maxLength={120} aria-invalid={!!nameError} aria-describedby="c-name-note" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <FieldNote id="c-name-note" error={nameError && t(nameError)} />
              <IconPicker value={icon} onChange={setIcon} label={t("collections.icon")} noneLabel={t("collections.icon_none")} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="c-desc">{t("collections.description")}</Label>
              <Textarea id="c-desc" rows={3} maxLength={2000} aria-invalid={!!descriptionRule(desc)} value={desc} onChange={(e) => setDesc(e.target.value)} />
            </div>
          </div>
        </FormSection>

        <FormSection title={t("collections.start_with")} description={t("collections.start.desc")} wide headAbove>
          <div className="space-y-4">
            <div role="radiogroup" aria-label={t("collections.start_with")} className="grid gap-3 sm:grid-cols-2">
              {STARTS.map(({ id, icon: Icon }) => (
                <button key={id} type="button" role="radio" aria-checked={start === id} onClick={() => { setStart(id); setError(null); }}
                  className={cn("flex flex-col items-start gap-1.5 rounded-lg border p-4 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    start === id && "border-primary bg-primary/5 ring-1 ring-primary hover:bg-primary/5")}>
                  <Icon className={cn("size-5", start === id ? "text-primary" : "text-muted-foreground")} aria-hidden />
                  <span className="text-sm font-medium">{t(`collections.start.${id}`)}</span>
                  <span className="text-xs text-muted-foreground">{t(`collections.start.${id}_desc`)}</span>
                </button>
              ))}
            </div>

            <div className="rounded-lg border bg-muted/30 p-4">
              {start === "scratch" && (
                <div className="space-y-3">
                  <div className="space-y-2">
                    <Label htmlFor="c-template">{t("collections.start.template")}</Label>
                    <Select value={template} items={Object.fromEntries(TEMPLATE_IDS.map((id) => [id, t(`collections.template.${id}`)]))} onValueChange={(v) => v && setTemplate(v as TemplateId)}>
                      <SelectTrigger id="c-template" className="w-64 max-w-full"><SelectValue /></SelectTrigger>
                      <SelectContent>{TEMPLATE_IDS.map((id) => <SelectItem key={id} value={id}>{t(`collections.template.${id}`)}</SelectItem>)}</SelectContent>
                    </Select>
                  </div>
                  <FieldList titles={fieldTitles(schema)} empty={t("collections.start.no_fields")} />
                  <p className="text-xs text-muted-foreground">{t("collections.start.scratch_after")}</p>
                </div>
              )}

              {start === "file" && (
                <div className="space-y-3">
                  <div className="flex flex-wrap items-center gap-3">
                    <Button type="button" variant="outline" onClick={() => fileInput.current?.click()}><FileUp className="size-4" />{t(file ? "collections.start.file_other" : "collections.start.file_pick")}</Button>
                    <input ref={fileInput} type="file" accept={DATA_FILES} className="hidden" aria-label={t("collections.start.file")}
                      onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ""; if (f) void pickFile(f); }} />
                    <p className="text-sm text-muted-foreground" role="status">{fileNote ?? t("collections.template_file_hint")}</p>
                  </div>
                  {file && <FieldList titles={fieldTitles(file.schema)} empty={t("collections.start.no_fields")} />}
                  <label className="flex items-center gap-2 text-sm">
                    <Checkbox checked={importRows} onCheckedChange={(v) => setImportRows(v === true)} />
                    {t("collections.start.import_rows")}
                  </label>
                </div>
              )}

            </div>
          </div>
        </FormSection>

        <SaveBar
          footer
          submit
          label={t("collections.new")}
          status={
            <span className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              {t(!name.trim() ? "collections.name_needed" : missing ? `collections.start.${start}_missing` : "collections.ready_to_create")}
              <span className="hidden text-xs font-normal text-muted-foreground sm:inline"><kbd className="font-mono">Ctrl</kbd>+<kbd className="font-mono">Enter</kbd> {t("records.shortcut_save")}</span>
            </span>
          }
          error={error}
          saving={create.isPending}
          // Create stays live without a name: pressing it says what is missing.
          canSave={!missing && !descriptionRule(desc) && !collectionName(name)}
          saveLabel={t("common.create")}
          discardLabel={t("common.cancel")}
          onDiscard={() => router.push("/collections")}
        />
      </FormSections>
    </form>
  );
}

/** The fields a start brings, as names: a glance before creating, edited later on Fields. */
function FieldList({ titles, empty }: { titles: string[]; empty: string }) {
  const { t } = useI18n();
  if (!titles.length) return <p className="text-sm text-muted-foreground">{empty}</p>;
  return (
    <div className="space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">{plural(t("collections.start.fields_count"), titles.length)}</p>
      <ul className="flex flex-wrap gap-1.5">
        {titles.map((n, i) => <li key={i} className="rounded-md border bg-background px-2 py-0.5 text-xs">{n}</li>)}
      </ul>
    </div>
  );
}
