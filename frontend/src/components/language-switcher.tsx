"use client";

import { Check, Languages } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useI18n } from "@/lib/i18n/provider";
import { LOCALES } from "@/lib/i18n/messages";
import { useOptionalMe } from "@/lib/context/me";

export function LanguageSwitcher() {
  const { locale, setLocale, t } = useI18n();
  const me = useOptionalMe();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger aria-label={t("common.language")} title={t("common.language")} className="flex size-11 md:size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground">
        <Languages className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t("common.language")}</DropdownMenuLabel>
          {LOCALES.map((l) => (
            <DropdownMenuItem key={l.value} onClick={() => {
                setLocale(l.value);
                void me?.savePreferences({ language: l.value });
              }} className="gap-2">
              <span>{l.flag}</span>
              <span className="flex-1">{l.label}</span>
              {locale === l.value && <Check className="size-4" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
