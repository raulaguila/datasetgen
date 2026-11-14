"use client";

import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import { Loader2, Monitor, Smartphone } from "lucide-react";
import { toast } from "sonner";
import { FieldNote, FormError, FormSection, FormSections, PageBody, SaveBar, PageHeader, initials, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { PasswordInput } from "@/components/password-input";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { ApiError } from "@/lib/api/client";
import { account } from "@/lib/api/endpoints";
import type { Language, Theme } from "@/lib/api/types";
import { ME_KEY, useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { LOCALES } from "@/lib/i18n/messages";
import type { MessageKey } from "@/lib/i18n/messages";
import { describeDevice } from "@/lib/device";
import { useI18n } from "@/lib/i18n/provider";
import { newPassword, personName, ready, sameAs } from "@/lib/validation";
import { squareImage } from "@/lib/image";
import { passwordScore } from "@/lib/password";
import { cn } from "@/lib/utils";

function ProfileHeader() {
  const { t, locale } = useI18n();
  const { user } = useMe();
  const client = useQueryClient();
  const picker = useRef<HTMLInputElement>(null);
  const done = (key: MessageKey) => {
    toast.success(t(key));
    void client.invalidateQueries({ queryKey: ME_KEY });
  };
  const upload = useMutation({
    mutationFn: async (file: File) => {
      let image: Blob;
      try {
        image = await squareImage(file);
      } catch {
        throw new Error(t("profile.avatar_unreadable"));
      }
      return account.setAvatar(image);
    },
    onSuccess: () => done("profile.avatar_saved"),
    onError: (err) => toast.error(err instanceof ApiError ? errorMessage(err, "common.save_failed", t) : err.message),
  });
  const remove = useMutation({
    mutationFn: account.removeAvatar,
    onSuccess: () => done("profile.avatar_removed"),
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  if (!user) return null;
  const busy = upload.isPending || remove.isPending;
  return (
    <div className="flex flex-wrap items-center gap-4">
      <Avatar className="size-14">
        {user.avatar_at && <AvatarImage src={account.avatarUrl(user.avatar_at)} alt="" />}
        <AvatarFallback className="text-lg">{initials(user.name)}</AvatarFallback>
      </Avatar>
      <div className="min-w-0 flex-1">
        <div className="truncate text-lg font-semibold">{user.name}</div>
        <div className="text-sm break-all text-muted-foreground">{user.email}</div>
        <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
          {/* The two roles the platform ships with read in the app language; others keep the name an admin gave them. */}
          {user.roles.map((r) => <Badge key={r} variant="secondary">{r === "admin" ? t("roles.builtin.admin") : r === "user" ? t("roles.builtin.user") : r}</Badge>)}
          <span>{t("profile.member_since")} {new Date(user.created_at).toLocaleDateString(locale, { dateStyle: "medium" })}</span>
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        <input
          ref={picker}
          type="file"
          accept="image/png,image/jpeg,image/webp"
          className="sr-only"
          tabIndex={-1}
          aria-hidden
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = ""; // picking the same file again still fires
            if (file) upload.mutate(file);
          }}
        />
        <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => picker.current?.click()}>
          {upload.isPending && <Loader2 className="size-4 animate-spin" />}
          {t(user.avatar_at ? "profile.avatar_change" : "profile.avatar_add")}
        </Button>
        {user.avatar_at && (
          <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => remove.mutate()}>
            {t("profile.avatar_remove")}
          </Button>
        )}
      </div>
    </div>
  );
}

const STRENGTH: MessageKey[] = ["password.weak", "password.weak", "password.fair", "password.good", "password.strong"];
const STRENGTH_CLS = ["bg-destructive", "bg-destructive", "bg-warn", "bg-success/70", "bg-success"];

function StrengthMeter({ password }: { password: string }) {
  const { t } = useI18n();
  if (!password) return null;
  const score = passwordScore(password);
  return (
    <div className="flex items-center gap-2" aria-live="polite">
      <div className="flex flex-1 gap-1" aria-hidden>
        {[1, 2, 3, 4].map((i) => <div key={i} className={cn("h-1 flex-1 rounded-full bg-muted", i <= Math.max(score, 1) && STRENGTH_CLS[score])} />)}
      </div>
      <span className="w-16 text-right text-xs text-muted-foreground">
        <span className="sr-only">{t("password.strength")}: </span>{t(STRENGTH[score])}
      </span>
    </div>
  );
}

function NameCard() {
  const { t } = useI18n();
  const { user } = useMe();
  const client = useQueryClient();
  const [name, setName] = useState(user?.name ?? "");
  const [error, setError] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: () => account.rename(name),
    onSuccess: (data) => {
      client.setQueryData(ME_KEY, data);
      setError(null);
      toast.success(t("profile.name_saved"));
    },
    onError: (err) => setError(errorMessage(err, "profile.save_failed", t)),
  });
  const dirty = name.trim() !== user?.name;
  return (
    <FormSection title={t("profile.account")} description={t("profile.account_desc")}>
    <form onSubmit={(e) => { e.preventDefault(); save.mutate(); }} className="space-y-6">
          <div className="space-y-2">
            <Label htmlFor="name">{t("auth.name")}</Label>
            <Input id="name" required maxLength={80} aria-invalid={!!personName(name)} aria-describedby="name-note" value={name} onChange={(e) => setName(e.target.value)} />
            <FieldNote id="name-note" error={personName(name) && t(personName(name)!)} />
          </div>
    {dirty && (
      <SaveBar
        submit
        label={t("roles.unsaved")}
        error={error}
        saving={save.isPending}
        canSave={ready([name, personName])}
        saveLabel={t("common.save")}
        discardLabel={t("common.discard")}
        onDiscard={() => { setName(user?.name ?? ""); setError(null); }}
      />
    )}
    </form>
    </FormSection>
  );
}

function PasswordCard() {
  const { t } = useI18n();
  const client = useQueryClient();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);
  const change = useMutation({
    mutationFn: () => account.changePassword(current, next),
    onSuccess: () => {
      setCurrent("");
      setNext("");
      setConfirm("");
      setError(null);
      void client.invalidateQueries({ queryKey: SESSIONS_KEY });
      toast.success(t("profile.password_changed"));
    },
    onError: (err) => setError(errorMessage(err, "profile.save_failed", t)),
  });
  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    change.mutate();
  };
  return (
    <FormSection title={t("profile.password")} description={t("profile.password_hint")}>
        <form className="space-y-4" onSubmit={submit}>
          <FormError message={error} />
          <div className="space-y-2">
            <Label htmlFor="current">{t("profile.current_password")}</Label>
            <PasswordInput id="current" autoComplete="current-password" required value={current} onChange={(e) => setCurrent(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new">{t("auth.new_password")}</Label>
            <PasswordInput id="new" autoComplete="new-password" required maxLength={128} aria-invalid={!!newPassword(next)} aria-describedby="new-note" value={next} onChange={(e) => setNext(e.target.value)} />
            <StrengthMeter password={next} />
            <FieldNote id="new-note" hint={t("auth.password_rule")} error={newPassword(next) && t("auth.password_rule")} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="confirm">{t("auth.confirm_password")}</Label>
            <PasswordInput id="confirm" autoComplete="new-password" required aria-invalid={!!sameAs(next)(confirm)} aria-describedby="confirm-note" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
            <FieldNote id="confirm-note" error={sameAs(next)(confirm) && t("auth.password_mismatch")} />
          </div>
          {/* Changing the password is an action, not a draft: it keeps its own button. */}
          <div className="flex justify-end">
            <Button type="submit" disabled={change.isPending || !ready([current], [next, newPassword], [confirm, sameAs(next)])}>
              {change.isPending && <Loader2 className="size-4 animate-spin" />}
              {t("profile.change_password")}
            </Button>
          </div>
        </form>
    </FormSection>
  );
}

function PreferencesCard() {
  const { t, setLocale } = useI18n();
  const { setTheme } = useTheme();
  const { user, savePreferences } = useMe();
  const save = (p: Parameters<typeof savePreferences>[0]) =>
    void savePreferences(p).then((ok) => (ok ? toast.success(t("common.saved")) : toast.error(t("profile.save_failed"))));
  const themes: Record<Theme, string> = { system: t("profile.theme_system"), light: t("profile.theme_light"), dark: t("profile.theme_dark") };
  const languages = Object.fromEntries(LOCALES.map((l) => [l.value, l.label])) as Record<Language, string>;
  if (!user) return null;
  return (
    <FormSection title={t("profile.preferences")} description={t("profile.preferences_desc")}>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label id="pref-theme">{t("common.theme")}</Label>
          <Select
            value={user.preferences.theme}
            items={themes}
            onValueChange={(v) => {
              setTheme(v as Theme);
              save({ theme: v as Theme });
            }}
          >
            <SelectTrigger className="w-full" aria-labelledby="pref-theme"><SelectValue /></SelectTrigger>
            <SelectContent>
              {Object.entries(themes).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label id="pref-language">{t("common.language")}</Label>
          <Select
            value={user.preferences.language}
            items={languages}
            onValueChange={(v) => {
              setLocale(v as Language);
              save({ language: v as Language });
            }}
          >
            <SelectTrigger className="w-full" aria-labelledby="pref-language"><SelectValue /></SelectTrigger>
            <SelectContent>
              {LOCALES.map((l) => <SelectItem key={l.value} value={l.value}>{l.label}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
      </div>
    </FormSection>
  );
}

const SESSIONS_KEY = ["sessions"] as const;

function SessionsCard() {
  const { t } = useI18n();
  const format = useFormatDate();
  const client = useQueryClient();
  const sessions = useQuery({ queryKey: SESSIONS_KEY, queryFn: account.sessions });
  const [confirmOthers, setConfirmOthers] = useState(false);
  const refresh = () => client.invalidateQueries({ queryKey: SESSIONS_KEY });
  const revoke = useMutation({
    mutationFn: account.revokeSession,
    onSuccess: () => void refresh(),
    onError: (err) => toast.error(errorMessage(err, "profile.save_failed", t)),
  });
  const others = sessions.data?.items.filter((s) => !s.current).length ?? 0;

  return (
    <FormSection title={t("profile.sessions")} description={t("profile.sessions_hint")}>
    <div>
      <div className="space-y-3">
        {others > 0 && <div className="flex justify-end"><Button variant="outline" size="sm" onClick={() => setConfirmOthers(true)}>{t("profile.revoke_others")}</Button></div>}
        {sessions.isPending ? (
          <ListSkeleton rows={2} />
        ) : sessions.isError ? (
          <QueryErrorState onRetry={() => void sessions.refetch()} />
        ) : (
          <ul className="divide-y rounded-lg border">
            {sessions.data.items.toSorted((a, b) => Number(b.current) - Number(a.current)).map((s) => (
              <li key={s.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                {(() => {
                  const device = describeDevice(s.user_agent);
                  const Icon = device.mobile ? Smartphone : Monitor;
                  return (
                    <>
                      <Icon className="size-4 shrink-0 text-muted-foreground" />
                      <div className="min-w-0 flex-1">
                        <div className="truncate text-sm font-medium" title={s.user_agent ?? undefined}>{device.label ?? t("profile.unknown_device")}</div>
                        <div className="text-xs text-muted-foreground">
                          {s.ip_address ?? "-"} · {t("profile.last_seen")} {format(s.last_seen_at)}
                        </div>
                      </div>
                    </>
                  );
                })()}
                {s.current ? (
                  <Badge variant="secondary">{t("profile.this_device")}</Badge>
                ) : (
                  <Button variant="ghost" size="sm" disabled={revoke.isPending} onClick={() => revoke.mutate(s.id)}>{t("profile.revoke")}</Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
      <ConfirmDialog
        open={confirmOthers}
        onOpenChange={setConfirmOthers}
        title={t("profile.revoke_others")}
        description={t("profile.revoke_others_desc")}
        destructive
        onConfirm={async () => {
          await account.revokeOtherSessions();
          await refresh();
        }}
      />
    </div>
    </FormSection>
  );
}

export default function ProfilePage() {
  const { t } = useI18n();
  const { user } = useMe();
  return (
    <>
      <PageHeader title={t("nav.profile")} description={t("profile.subtitle")} />
      <PageBody>
        {user ? (
          <>
            <ProfileHeader />
            <FormSections>
              <NameCard />
              <PasswordCard />
              <PreferencesCard />
              <SessionsCard />
            </FormSections>
          </>
        ) : (
          <ListSkeleton rows={4} />
        )}
      </PageBody>
    </>
  );
}
