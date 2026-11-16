"use client";

import { useSyncExternalStore } from "react";
import { Tabs } from "@base-ui/react/tabs";

const onHash = (cb: () => void) => { window.addEventListener("hashchange", cb); return () => window.removeEventListener("hashchange", cb); };

/** A page's tabs; the address hash (#keys) opens a tab directly. The first tab is the default. */
export function HashTabs({ label, tabs }: { label: string; tabs: { id: string; title: string; panel: React.ReactNode }[] }) {
  const hash = useSyncExternalStore(onHash, () => window.location.hash.slice(1), () => "");
  const tab = tabs.some((x) => x.id === hash) ? hash : tabs[0].id;
  const pick = (v: string) => {
    history.replaceState(null, "", `#${v}`);
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  };
  return (
    <Tabs.Root value={tab} onValueChange={(v) => pick(String(v))}>
      <Tabs.List aria-label={label} className="mb-6 flex gap-1 overflow-x-auto border-b">
        {tabs.map((x) => (
          <Tabs.Tab key={x.id} value={x.id} className="-mb-px shrink-0 whitespace-nowrap border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-[active]:border-primary data-[active]:font-medium data-[active]:text-foreground">
            {x.title}
          </Tabs.Tab>
        ))}
      </Tabs.List>
      {tabs.map((x) => <Tabs.Panel key={x.id} value={x.id} className="outline-none">{x.panel}</Tabs.Panel>)}
    </Tabs.Root>
  );
}
