"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2, UserPlus, Users } from "lucide-react";
import { toast } from "sonner";
import { FieldNote, FormSection, FormSections, IconAction, initials, avatarStyle } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ApiError } from "@/lib/api/client";
import { collections } from "@/lib/api/endpoints";
import type { Collection, CollectionMember, MemberRole } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { email as emailRule, personName } from "@/lib/validation";
import { CollectionPage } from "../_parts";

const ROLES: MemberRole[] = ["owner", "editor", "viewer"];

function useRoleItems() {
  const { t } = useI18n();
  return Object.fromEntries(ROLES.map((r) => [r, t(`members.role.${r}`)]));
}

function RoleSelect({ value, label, disabled, onChange }: { value: MemberRole; label: string; disabled?: boolean; onChange: (r: MemberRole) => void }) {
  const { t } = useI18n();
  const items = useRoleItems();
  return (
    <Select value={value} items={items} disabled={disabled} onValueChange={(v) => onChange(v as MemberRole)}>
      <SelectTrigger className="w-36" aria-label={label}><SelectValue /></SelectTrigger>
      <SelectContent>
        {ROLES.map((r) => (
          <SelectItem key={r} value={r}>
            <span className="flex flex-col">
              <span>{items[r]}</span>
              <span className="text-xs text-muted-foreground">{t(`members.role.${r}_desc`)}</span>
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Adds a person by email; an email without an account is invited and joins at once. */
function AddMember({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState<MemberRole>("editor");
  const [needName, setNeedName] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [link, setLink] = useState<string | null>(null);
  const add = useMutation({
    mutationFn: () => collections.addMember(collection.id, { email: email.trim(), name: name.trim() || undefined, role }),
    onSuccess: (res) => {
      void client.invalidateQueries({ queryKey: ["collections", "members", collection.id] });
      toast.success(t(res.invited ? "members.invited" : "members.added"));
      // Only a development backend hands the token back (manual delivery).
      setLink(res.activation_token ? `${window.location.origin}/activate#token=${encodeURIComponent(res.activation_token)}` : null);
      setEmail("");
      setName("");
      setNeedName(false);
      setError(null);
    },
    onError: (err) => {
      // No account has this email: ask for a name and send the invite with it.
      if (err instanceof ApiError && err.field === "name" && !name.trim()) {
        setNeedName(true); // the name field's hint says why
        return;
      }
      setError(errorMessage(err, "common.save_failed", t));
    },
  });
  const nameRule = needName || name.trim() ? personName(name) : undefined;
  const canAdd = !emailRule(email) && email.trim() !== "" && !nameRule && (!needName || name.trim() !== "");

  return (
    <FormSections>
      <FormSection title={t("members.add")} description={t("members.add_desc")}>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (canAdd) add.mutate(); }}>
          <div className="space-y-2">
            <Label htmlFor="m-email">{t("auth.email")}</Label>
            <Input id="m-email" type="email" required maxLength={254} aria-invalid={!!(email && emailRule(email))} aria-describedby="m-email-note" value={email}
              onChange={(e) => { setEmail(e.target.value); setNeedName(false); setError(null); }} />
            <FieldNote id="m-email-note" error={email && emailRule(email) ? t(emailRule(email)!) : undefined} />
          </div>
          {needName && (
            <div className="space-y-2">
              <Label htmlFor="m-name">{t("auth.name")}</Label>
              <Input id="m-name" autoFocus maxLength={80} aria-invalid={!!nameRule} aria-describedby="m-name-note" value={name} onChange={(e) => setName(e.target.value)} />
              <FieldNote id="m-name-note" error={nameRule && t(nameRule)} hint={t("members.name_hint")} />
            </div>
          )}
          <div className="flex flex-wrap items-end gap-3">
            <div className="space-y-2">
              <Label>{t("members.role")}</Label>
              <RoleSelect value={role} label={t("members.role")} onChange={setRole} />
            </div>
            <Button type="submit" disabled={!canAdd || add.isPending}><UserPlus className="size-4" />{t(needName ? "members.invite" : "members.add")}</Button>
          </div>
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          {link && (
            <div className="space-y-2 rounded-lg border bg-muted/30 p-3 text-sm">
              <p>{t("users.activation_link")}</p>
              <div className="flex gap-2">
                <Input readOnly aria-label={t("users.link_title")} value={link} className="font-mono text-xs" onFocus={(e) => e.target.select()} />
                <Button type="button" variant="outline" onClick={() => void navigator.clipboard.writeText(link).then(() => toast.success(t("common.copied")))}>{t("common.copy")}</Button>
              </div>
            </div>
          )}
        </form>
      </FormSection>
    </FormSections>
  );
}

function MemberList({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const { user } = useMe();
  const client = useQueryClient();
  const [removing, setRemoving] = useState<CollectionMember | null>(null);
  const key = ["collections", "members", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => collections.members(collection.id) });
  const refresh = () => {
    void client.invalidateQueries({ queryKey: key });
    // The caller's own role may have changed: the sidebar and pages follow it.
    void client.invalidateQueries({ queryKey: ["collections"] });
  };
  const setRole = useMutation({
    mutationFn: ({ m, role }: { m: CollectionMember; role: MemberRole }) => collections.setMemberRole(collection.id, m.user_id, role),
    onSuccess: () => { toast.success(t("common.saved")); refresh(); },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });

  if (list.isPending) return <ListSkeleton rows={3} />;
  if (list.isError) return <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />;
  const items = list.data.items;
  if (items.length === 0) return <EmptyState icon={Users} title={t("members.empty")} />;
  return (
    <Card className="gap-0 overflow-hidden p-0">
      <ul aria-label={t("collections.tab.members")} className="divide-y">
        {items.map((m) => (
          <li key={m.user_id} className="flex flex-wrap items-center gap-3 px-4 py-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-full text-xs font-semibold" style={avatarStyle(m.user_id)} aria-hidden>{initials(m.name)}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {m.name}
                {m.user_id === user?.id && <span className="text-muted-foreground"> ({t("members.you")})</span>}
                {m.status === "pending" && <Badge variant="outline" className="ml-2">{t("members.pending")}</Badge>}
              </p>
              <p className="truncate text-xs text-muted-foreground">{m.email}</p>
            </div>
            <RoleSelect value={m.role} label={`${t("members.role")}: ${m.name}`} disabled={setRole.isPending} onChange={(role) => role !== m.role && setRole.mutate({ m, role })} />
            <IconAction label={t("members.remove")} target={m.name} className="text-destructive" onClick={() => setRemoving(m)}><Trash2 className="size-4" /></IconAction>
          </li>
        ))}
      </ul>
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("members.remove")}
        description={t("members.remove_confirm").replace("{name}", removing?.name ?? "")}
        destructive
        confirmLabel={t("members.remove")}
        onConfirm={async () => {
          if (!removing) return;
          await collections.removeMember(collection.id, removing.user_id); // the dialog shows a refusal, such as the last owner
          setRemoving(null);
          refresh();
        }}
      />
    </Card>
  );
}

export default function CollectionMembersPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.members">
      {(collection) => roleAllows(collection.role, "owner") ? (
        <div className="space-y-6">
          <AddMember collection={collection} />
          <MemberList collection={collection} />
        </div>
      ) : (
        <EmptyState icon={Users} title={t("error.forbidden")} description={t("members.owners_only")} />
      )}
    </CollectionPage>
  );
}
