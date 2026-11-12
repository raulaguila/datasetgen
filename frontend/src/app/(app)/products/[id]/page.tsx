"use client";

import Link from "next/link";
import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { DangerZone, FieldNote, FormSection, FormSections, PageBody, PageHeader, SaveBar, useFormatDate } from "@/components/bits";
import { CURRENCY } from "@/lib/brand";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { ApiError } from "@/lib/api/client";
import { products } from "@/lib/api/endpoints";
import type { Product } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useI18n } from "@/lib/i18n/provider";
import { price as priceRule, productName, ready } from "@/lib/validation";
import { can } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";

/** Create when `product` is null, edit otherwise. Price stays a decimal string end to end. */
function ProductForm({ product, editable }: { product: Product | null; editable: boolean }) {
  const { t, locale } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  // Shown with the language's decimal mark; sent with a dot.
  const comma = (1.5).toLocaleString(locale).includes(",");
  const symbol = new Intl.NumberFormat(locale, { style: "currency", currency: CURRENCY }).formatToParts(0).find((p) => p.type === "currency")?.value;
  const [name, setName] = useState(product?.name ?? "");
  const [price, setPrice] = useState(product ? (comma ? product.price.replace(".", ",") : product.price) : "");
  const sentPrice = price.trim().replace(",", ".");
  const [enabled, setEnabled] = useState(product?.enabled ?? true);
  const [error, setError] = useState<string | null>(null);
  // The version this form was loaded from: a save made over someone else's newer change is refused.
  const [version, setVersion] = useState(product?.version);
  const dirty = product
    ? name.trim() !== product.name || sentPrice !== product.price || enabled !== product.enabled
    : name.trim() !== "" || price.trim() !== "";
  useUnsavedGuard(editable && dirty, t("roles.leave_unsaved"));
  const save = useMutation({
    mutationFn: () => {
      const input = { name: name.trim(), price: sentPrice, enabled };
      return product ? products.update(product.id, { ...input, version }) : products.create(input);
    },
    onSuccess: (saved) => {
      setError(null);
      client.setQueryData(["products", "detail", saved.id], saved);
      void client.invalidateQueries({ queryKey: ["products"] });
      toast.success(t("common.saved"));
      router.push("/products");
    },
    onError: (err) => {
      setError(errorMessage(err, "common.save_failed", t));
      // Fetch the newer row now, so Discard shows it.
      if (product && err instanceof ApiError && err.code === "CONFLICT") void client.invalidateQueries({ queryKey: ["products", "detail", product.id] });
    },
  });

  const discard = () => {
    if (!product) return router.push("/products");
    setName(product.name);
    setPrice(comma ? product.price.replace(".", ",") : product.price);
    setEnabled(product.enabled);
    setVersion(product.version);
    setError(null);
  };

  return (
    <form onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <FormSections>
        <FormSection title={t("products.details")} description={t("products.details_desc")}>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="p-name">{t("auth.name")}</Label>
                <Input id="p-name" required autoFocus={!product} maxLength={120} aria-invalid={!!productName(name)} aria-describedby="p-name-note" disabled={!editable} value={name} onChange={(e) => setName(e.target.value)} />
                <FieldNote id="p-name-note" error={productName(name) && t(productName(name)!)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="p-price">{t("products.price")}</Label>
                <div className="relative max-w-48">
                  <span className="pointer-events-none absolute inset-y-0 left-3 flex items-center text-sm text-muted-foreground" aria-hidden="true">{symbol}</span>
                  <Input id="p-price" required inputMode="decimal" aria-invalid={!!priceRule(price)} aria-describedby="p-price-note" placeholder={comma ? "0,00" : "0.00"} disabled={!editable} value={price} onChange={(e) => setPrice(e.target.value)} className="pl-10 font-mono" />
                </div>
                <FieldNote id="p-price-note" error={priceRule(price) && t(priceRule(price)!)} />
              </div>
            </div>
        </FormSection>
        <FormSection title={t("products.status")} description={t("products.status_desc")}>
              <label className="flex items-center gap-3 text-sm">
                <Switch checked={enabled} disabled={!editable} onCheckedChange={setEnabled} aria-label={t("products.enabled")} />
                {t(enabled ? "roles.enabled" : "roles.disabled")}
              </label>
        </FormSection>
        {editable && (dirty || !product) && (
          <SaveBar
            footer
            submit
            label={t(product ? "roles.unsaved" : "products.new")}
            error={error}
            saving={save.isPending}
            canSave={dirty && ready([name, productName], [price, priceRule])}
            saveLabel={t(product ? "common.save" : "common.create")}
            discardLabel={t(product ? "common.discard" : "common.cancel")}
            onDiscard={discard}
          />
        )}
      </FormSections>
    </form>
  );
}

export default function ProductPage() {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const { user, loading } = useMe();
  const { id } = useParams<{ id: string }>();
  const creating = id === "new";
  const detail = useQuery({ queryKey: ["products", "detail", id], queryFn: () => products.get(id), enabled: !creating });
  const product = detail.data;
  const [confirmDelete, setConfirmDelete] = useState(false);
  const editable = can(user, "products", creating ? "create" : "update");
  useCrumb(creating ? t("products.new") : product?.name);
  const format = useFormatDate();
  const subtitle = product && `${t("common.created_at")} ${format(product.created_at)} · ${t("common.updated_at").toLowerCase()} ${format(product.updated_at)}`;

  return (
    <>
      <PageHeader
        title={creating ? t("products.new") : (product?.name ?? t("nav.products"))}
        description={subtitle || undefined}
      />
      <PageBody>
        {loading ? (
          <ListSkeleton rows={3} />
        ) : creating ? (
          <ProductForm product={null} editable={editable} />
        ) : detail.isPending ? (
          <ListSkeleton rows={3} />
        ) : detail.error instanceof ApiError && detail.error.code === "NOT_FOUND" ? (
          <EmptyState title={t("error.not_found")} description={t("error.record_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/products" />}>{t("common.back_to_list")}</Button>} />
        ) : detail.isError || !product ? (
          <QueryErrorState error={detail.error} onRetry={() => void detail.refetch()} />
        ) : (
          <ProductForm key={product.id} product={product} editable={editable} />
        )}
        {product && can(user, "products", "delete") && (
          <DangerZone description={t("products.delete_hint")} label={t("products.delete")} onClick={() => setConfirmDelete(true)} />
        )}
      </PageBody>
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={t("products.delete")}
        description={`${product?.name ?? ""}. ${t("products.delete_hint")}`}
        destructive
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          await products.remove([id]);
          router.push("/products");
          client.removeQueries({ queryKey: ["products", "detail", id] });
          void client.invalidateQueries({ queryKey: ["products"] });
        }}
      />
    </>
  );
}
