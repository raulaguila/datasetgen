"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Loader2, ShieldAlert } from "lucide-react";
import { toast } from "sonner";
import { ActiveBadge, DangerZone, FieldNote, FormError, FormSection, FormSections, SaveBar, PageBody, PageHeader, StatusBadge, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError } from "@/lib/api/client";
import { roles, users } from "@/lib/api/endpoints";
import type { UserDetails } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useMe } from "@/lib/context/me";
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useCrumb } from "@/lib/store/crumb";
import { useI18n } from "@/lib/i18n/provider";
import { email as emailRule, personName, ready } from "@/lib/validation";
import { can } from "@/lib/permissions";

/** Check boxes for every role the API lists; Save replaces the user's roles in one call. */
function RolesCard({ user }: { user: UserDetails }) {
  const { t } = useI18n();
  const { user: me } = useMe();
  const client = useQueryClient();
  const router = useRouter();
  const manage = can(me, "roles", "read") && can(me, "roles", "update");
  // shortcut: shows the first 100 roles (the API page cap); add search here if a deployment grows past that.
  const all = useQuery({ queryKey: ["roles", "list", "all"], queryFn: () => roles.list({ page: 1, limit: 100 }), enabled: manage });
  const saved = user.roles.map((r) => r.id).sort();
  const [draft, setDraft] = useState(saved);
  const dirty = JSON.stringify([...draft].sort()) !== JSON.stringify(saved);
  useUnsavedGuard(dirty, t("roles.leave_unsaved"));
  const save = useMutation({
    mutationFn: () => users.setRoles(user.id, draft),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ["users", "detail", user.id] });
      void client.invalidateQueries({ queryKey: ["roles"] });
      if (user.id === me?.id) void client.invalidateQueries({ queryKey: ["me"] });
      toast.success(t("common.saved"));
      router.push("/admin/users");
    },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const toggle = (id: string, on: boolean) => setDraft((d) => (on ? [...d, id] : d.filter((x) => x !== id)));

  return (
    <>
    <FormSection title={t("nav.roles")} description={t(manage ? "users.roles_hint" : "users.roles_desc")}>
      <div className="space-y-4">
        {!manage ? (
          <div className="flex flex-wrap gap-1.5">
            {user.roles.length === 0 ? <span className="text-sm text-muted-foreground">-</span> : user.roles.map((r) => (
              <Badge key={r.id} variant="secondary" render={<Link href={`/admin/roles/${r.id}`} />}>{r.name}</Badge>
            ))}
          </div>
        ) : all.isPending ? (
          <ListSkeleton rows={3} />
        ) : all.isError ? (
          <QueryErrorState error={all.error} onRetry={() => void all.refetch()} />
        ) : (
          <>
            <ul className="divide-y overflow-hidden rounded-lg border">
              {all.data.items.map((r) => (
                <li key={r.id}>
                  <label className="flex cursor-pointer items-center gap-3 px-4 py-2.5 text-sm hover:bg-accent">
                    <Checkbox checked={draft.includes(r.id)} disabled={save.isPending} onCheckedChange={(on) => toggle(r.id, on === true)} />
                    <span className="min-w-0 flex-1 truncate font-medium">{r.name}</span>
                    {r.is_default && <Badge variant="outline" title={t("roles.default_hint")}>{t("roles.default")}</Badge>}
                    {!r.enabled && <ActiveBadge enabled={false} />}
                  </label>
                </li>
              ))}
            </ul>
            {user.status === "pending" && all.data.items.some((r) => r.is_default) && (
              <p className="text-xs text-muted-foreground">{t("users.default_on_activation")}</p>
            )}
          </>
        )}
      </div>
    </FormSection>
    {/* Last child of the page's FormSections, so it docks as the card footer like every other form. */}
    {dirty && (
      <SaveBar
        footer
        label={t("roles.unsaved")}
        saving={save.isPending}
        canSave
        saveLabel={t("common.save")}
        discardLabel={t("common.discard")}
        onSave={() => save.mutate()}
        onDiscard={() => setDraft(saved)}
      />
    )}
    </>
  );
}

/** Creates a pending account; the person gets the activation email. */
function InviteForm() {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [link, setLink] = useState<string | null>(null);
  useUnsavedGuard(!link && (name.trim() !== "" || email.trim() !== ""), t("roles.leave_unsaved"));
  const invite = useMutation({
    mutationFn: () => users.invite(name.trim(), email.trim()),
    onSuccess: (res) => {
      void client.invalidateQueries({ queryKey: ["users"] });
      toast.success(t("users.invited"));
      // Only a development backend hands the token back (manual delivery).
      if (res.activation_token) setLink(`${window.location.origin}/activate#token=${encodeURIComponent(res.activation_token)}`);
      else router.push("/admin/users");
    },
    onError: (err) => setError(errorMessage(err, "common.save_failed", t)),
  });

  if (link) {
    return (
      <FormSections>
      <FormSection title={t("users.link_title")} description={t("users.activation_link")}>
          <div className="space-y-4 text-sm">
            <Input readOnly aria-label={t("users.link_title")} value={link} className="font-mono text-xs" onFocus={(e) => e.target.select()} />
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => void navigator.clipboard.writeText(link).then(() => toast.success(t("common.copied")))}>{t("common.copy")}</Button>
              <Button onClick={() => router.push("/admin/users")}>{t("common.done")}</Button>
            </div>
          </div>
      </FormSection>
      </FormSections>
    );
  }

  return (
    <form onSubmit={(e) => { e.preventDefault(); invite.mutate(); }}>
      <FormSections>
        <FormSection title={t("users.invite_person")} description={t("users.invite_desc")}>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="i-name">{t("auth.name")}</Label>
                <Input id="i-name" required autoFocus maxLength={80} aria-invalid={!!personName(name)} aria-describedby="i-name-note" value={name} onChange={(e) => setName(e.target.value)} />
                <FieldNote id="i-name-note" error={personName(name) && t(personName(name)!)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="i-email">{t("auth.email")}</Label>
                <Input id="i-email" type="email" required maxLength={254} aria-invalid={!!emailRule(email)} aria-describedby="i-email-note" value={email} onChange={(e) => setEmail(e.target.value)} />
                <FieldNote id="i-email-note" error={emailRule(email) && t(emailRule(email)!)} />
              </div>
            </div>
        </FormSection>
        <SaveBar
          footer
          submit
          label={t("users.invite")}
          error={error}
          saving={invite.isPending}
          canSave={ready([name, personName], [email, emailRule])}
          saveLabel={t("users.send_invite")}
          discardLabel={t("common.cancel")}
          onDiscard={() => router.push("/admin/users")}
        />
      </FormSections>
    </form>
  );
}

export default function UserDetailPage() {
  const { id } = useParams<{ id: string }>();
  return id === "new" ? <InvitePage /> : <UserDetail id={id} />;
}

function InvitePage() {
  const { t } = useI18n();
  useCrumb(t("users.invite"));
  return (
    <>
      <PageHeader title={t("users.invite")} />
      <PageBody><InviteForm /></PageBody>
    </>
  );
}

function UserDetail({ id }: { id: string }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const { user: me } = useMe();
  const client = useQueryClient();
  const detail = useQuery({ queryKey: ["users", "detail", id], queryFn: () => users.get(id) });
  const [confirm, setConfirm] = useState(false);
  const [revoking, setRevoking] = useState(false);
  const [unlocking, setUnlocking] = useState(false);
  const [linking, setLinking] = useState(false);

  const u = detail.data;
  useCrumb(u?.name);
  const disabled = u?.status === "disabled";
  // The API refuses changing yourself; hide the button instead of showing an error.
  const canToggle = u && can(me, "users", "update") && u.id !== me?.id;
  const a = u?.access;
  const locked = !!a?.locked_until && new Date(a.locked_until) > new Date();
  const blocked = u?.status === "locked" || locked || (a?.recent_failures ?? 0) > 0;
  const reinvite = useMutation({
    mutationFn: () => users.reinvite(id),
    onSuccess: async (res) => {
      // Only a development backend hands the token back (manual delivery).
      void client.invalidateQueries({ queryKey: ["users", "detail", id] }); // the link's expiry moved
      if (!res.activation_token) return void toast.success(t("users.reinvited"));
      await navigator.clipboard.writeText(`${window.location.origin}/activate#token=${encodeURIComponent(res.activation_token)}`);
      toast.success(t("users.reinvited_link"));
    },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });

  const linkState = (expires: string | null) => {
    if (!expires) return <span className="text-muted-foreground">{t("users.link_none")}</span>;
    const expired = new Date(expires) <= new Date();
    const text = t(expired ? "users.link_expired" : "users.link_expires").replace("{date}", format(expires));
    return <span className={expired ? "text-destructive" : undefined}>{text}</span>;
  };

  return (
    <>
      <PageHeader
        title={u?.name ?? t("nav.users")}
        description={u?.email}
        action={u && (
          <div className="flex flex-wrap gap-2">
            {can(me, "audit", "read") && (
              <Button variant="outline" nativeButton={false} render={<Link href={`/admin/audit?user=${u.id}`} />}>{t("users.view_audit")}</Button>
            )}
            {canToggle && blocked && (
              <Button variant="outline" onClick={() => setUnlocking(true)}>{t("users.unlock")}</Button>
            )}
            {canToggle && u.status === "pending" && (
              <DropdownMenu>
                <DropdownMenuTrigger render={<Button variant="outline" disabled={reinvite.isPending} />}>
                  {t("users.activation")}<ChevronDown className="size-4" />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-auto min-w-48">
                  <DropdownMenuItem onClick={() => reinvite.mutate()}>{t("users.resend_email")}</DropdownMenuItem>
                  {can(me, "roles", "update") && <DropdownMenuItem onClick={() => setLinking(true)}>{t("users.copy_link")}</DropdownMenuItem>}
                </DropdownMenuContent>
              </DropdownMenu>
            )}
            {canToggle && (a?.active_sessions ?? 0) > 0 && (
              <Button variant="outline" onClick={() => setRevoking(true)}>{t("users.revoke_sessions")}</Button>
            )}
            {canToggle && disabled && (
              <Button onClick={() => setConfirm(true)}>{t("users.enable")}</Button>
            )}
          </div>
        )}
      />
      <PageBody>
        {detail.isPending ? (
          <ListSkeleton rows={3} />
        ) : detail.error instanceof ApiError && detail.error.code === "NOT_FOUND" ? (
          <EmptyState title={t("error.not_found")} description={t("error.record_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/admin/users" />}>{t("common.back_to_list")}</Button>} />
        ) : detail.isError ? (
          <QueryErrorState error={detail.error} onRetry={() => void detail.refetch()} />
        ) : (
          <>
          {blocked && (
            <div role="status" className="flex items-start gap-3 rounded-lg border border-warn/40 bg-warn/10 px-4 py-3 text-sm text-warn">
              <ShieldAlert className="mt-0.5 size-4 shrink-0" />
              <span>{t("users.login_blocked")}</span>
            </div>
          )}
          <FormSections>
          <FormSection title={t("users.details")} description={t("users.details_desc")}>
              <dl className="grid gap-x-6 gap-y-4 text-sm sm:grid-cols-[10rem_1fr]">
                <dt className="text-muted-foreground">{t("users.status")}</dt>
                <dd><StatusBadge status={detail.data.status} /></dd>
                <dt className="text-muted-foreground">{t("common.created_at")}</dt>
                <dd>{format(detail.data.created_at)}</dd>
                <dt className="text-muted-foreground">{t("common.updated_at")}</dt>
                <dd>{format(detail.data.updated_at)}</dd>
              </dl>
              {a && (
                <>
                  <h3 className="mt-6 mb-3 text-sm font-semibold">{t("users.access")}</h3>
                  <dl className="grid gap-x-6 gap-y-4 text-sm sm:grid-cols-[10rem_1fr]">
                    <dt className="text-muted-foreground">{t("users.last_login")}</dt>
                    <dd>{a.last_login_at ? format(a.last_login_at) : t("users.never")}</dd>
                    <dt className="text-muted-foreground">{t("users.active_sessions")}</dt>
                    <dd className="tabular-nums">{a.active_sessions}</dd>
                    <dt className="text-muted-foreground">{t("users.recent_failures")}</dt>
                    <dd className="tabular-nums" title={t("users.recent_failures_hint")}>{a.recent_failures}</dd>
                    {u?.status === "pending" && (
                      <>
                        <dt className="text-muted-foreground">{t("users.link_title")}</dt>
                        <dd>{linkState(a.activation_expires_at)}</dd>
                      </>
                    )}
                    {locked && (
                      <>
                        <dt className="text-muted-foreground">{t("users.locked_until")}</dt>
                        <dd>{format(a.locked_until!)}</dd>
                      </>
                    )}
                  </dl>
                </>
              )}
          </FormSection>
          <RolesCard user={detail.data} />
          </FormSections>
          {canToggle && !disabled && (
            <DangerZone description={t("users.disable_desc")} label={t("users.disable")} onClick={() => setConfirm(true)} />
          )}
          </>
        )}
      </PageBody>
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t(disabled ? "users.enable" : "users.disable")}
        description={t(disabled ? "users.enable_desc" : "users.disable_desc")}
        destructive={!disabled}
        onConfirm={async () => {
          await (disabled ? users.enable : users.disable)([id]);
          await client.invalidateQueries({ queryKey: ["users"] });
          toast.success(t("common.saved"));
        }}
      />
      <ConfirmDialog
        open={unlocking}
        onOpenChange={setUnlocking}
        title={t("users.unlock")}
        description={t("users.unlock_desc")}
        onConfirm={async () => {
          await users.unlock([id]);
          await client.invalidateQueries({ queryKey: ["users"] });
          toast.success(t("users.unlocked"));
        }}
      />
      <ActivationLinkDialog userId={id} open={linking} onOpenChange={setLinking} />
      <ConfirmDialog
        open={revoking}
        onOpenChange={setRevoking}
        title={t("users.revoke_sessions")}
        description={t("users.revoke_sessions_desc")}
        destructive
        onConfirm={async () => {
          await users.revokeSessions(id);
          toast.success(t("users.sessions_revoked"));
        }}
      />
    </>
  );
}

/** Makes a new activation link to hand over by hand; it is shown once, so the dialog holds it until closed. */
function ActivationLinkDialog({ userId, open, onOpenChange }: { userId: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const client = useQueryClient();
  const make = useMutation({
    mutationFn: () => users.activationLink(userId),
    onSuccess: () => void client.invalidateQueries({ queryKey: ["users", "detail", userId] }),
  });
  const link = make.data && `${window.location.origin}/activate#token=${encodeURIComponent(make.data.activation_token)}`;
  const change = (next: boolean) => {
    onOpenChange(next);
    if (!next) make.reset(); // the link is gone once the dialog closes
  };
  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("users.link_title")}</DialogTitle>
          <DialogDescription>{t(link ? "users.link_ready" : "users.link_desc")}</DialogDescription>
        </DialogHeader>
        <FormError message={make.error ? errorMessage(make.error, "common.save_failed", t) : null} />
        {link && (
          <div className="space-y-2">
            <Input readOnly aria-label={t("users.link_title")} value={link} className="font-mono text-xs" onFocus={(e) => e.target.select()} />
            <p className="text-xs text-muted-foreground">{t("users.link_expires").replace("{date}", format(make.data!.expires_at))}</p>
          </div>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={() => change(false)}>{t("common.close")}</Button>
          {link ? (
            <Button onClick={() => void navigator.clipboard.writeText(link).then(() => toast.success(t("common.copied")))}>{t("common.copy")}</Button>
          ) : (
            <Button disabled={make.isPending} onClick={() => make.mutate()}>
              {make.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("users.link_generate")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
