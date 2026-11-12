import type { Metadata } from "next";
import { cookies } from "next/headers";
import localFont from "next/font/local";
import "./globals.css";
import { Providers } from "@/components/providers";
import { APP_NAME } from "@/lib/brand";
import { LOCALE_COOKIE } from "@/lib/i18n/provider";

const ibmPlexSans = localFont({
  src: [{ path: "../../public/fonts/ibm-plex-sans-latin-variable.woff2", weight: "400 700", style: "normal" }],
  variable: "--font-ibm-plex-sans",
  display: "swap",
});

const ibmPlexMono = localFont({
  src: [
    { path: "./fonts/ibm-plex-mono-latin-400.woff2", weight: "400", style: "normal" },
    { path: "./fonts/ibm-plex-mono-latin-500.woff2", weight: "500", style: "normal" },
  ],
  variable: "--font-ibm-plex-mono",
  display: "swap",
  preload: false, // only prices and codes inside the app use it; the sign-in pages need not wait for it
});

export const metadata: Metadata = {
  icons: { icon: [{ url: "/favicon.svg", type: "image/svg+xml" }] },
};

// Density, radius and the collapsed sidebar apply before paint, so the first
// frame matches the saved choice.
const prePaint = `(function(){try{var r=localStorage.getItem('ui');if(r&&JSON.parse(r).state.sidebarCollapsed)document.documentElement.classList.add('sidebar-collapsed')}catch(e){}try{var d=localStorage.getItem('density');if(d==='comfortable'||d==='compact')document.documentElement.setAttribute('data-density',d);var a=localStorage.getItem('radius');if(a==='sharp'||a==='subtle'||a==='default'||a==='rounded')document.documentElement.setAttribute('data-radius',a)}catch(e){}})();`;

export default async function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  const locale = (await cookies()).get(LOCALE_COOKIE)?.value === "en-US" ? "en-US" : "pt-BR";
  return (
    <html lang={locale} suppressHydrationWarning className={`${ibmPlexSans.variable} ${ibmPlexMono.variable} h-full antialiased`}>
      <head>
        {/* Not in metadata: Next would reset it on every navigation over the per-screen title (useDocumentTitle). */}
        <title>{APP_NAME}</title>
        <script dangerouslySetInnerHTML={{ __html: prePaint }} />
      </head>
      <body className="flex min-h-full flex-col font-sans">
        <Providers initialLocale={locale}>{children}</Providers>
      </body>
    </html>
  );
}
