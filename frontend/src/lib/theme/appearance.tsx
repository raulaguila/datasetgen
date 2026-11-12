"use client";

import { createContext, useCallback, useContext, useEffect, useState } from "react";

export type Density = "comfortable" | "compact";
export type RadiusPref = "sharp" | "subtle" | "default" | "rounded";

export const DENSITIES: Density[] = ["comfortable", "compact"];
export const RADII: RadiusPref[] = ["sharp", "subtle", "default", "rounded"];

const DENSITY_KEY = "density";
const RADIUS_KEY = "radius";

export const DEFAULT_DENSITY: Density = "comfortable";
export const DEFAULT_RADIUS: RadiusPref = "default";

function applyDensity(d: Density) {
  document.documentElement.setAttribute("data-density", d);
}

function applyRadius(r: RadiusPref) {
  document.documentElement.setAttribute("data-radius", r);
}

interface AppearanceCtx {
  density: Density;
  radius: RadiusPref;
  setDensity: (d: Density) => void;
  setRadius: (r: RadiusPref) => void;
}

const Ctx = createContext<AppearanceCtx | null>(null);

export function AppearanceProvider({ children }: { children: React.ReactNode }) {
  const [density, setDensityState] = useState<Density>(DEFAULT_DENSITY);
  const [radius, setRadiusState] = useState<RadiusPref>(DEFAULT_RADIUS);

  useEffect(() => {
    const d = localStorage.getItem(DENSITY_KEY);
    const r = localStorage.getItem(RADIUS_KEY);
    if (d === "comfortable" || d === "compact") {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- hydrate after mount
      setDensityState(d);
      applyDensity(d);
    } else {
      applyDensity(DEFAULT_DENSITY);
    }
    if (r === "sharp" || r === "subtle" || r === "default" || r === "rounded") {
      setRadiusState(r);
      applyRadius(r);
    } else {
      applyRadius(DEFAULT_RADIUS);
    }
  }, []);

  const setDensity = useCallback((d: Density) => {
    setDensityState(d);
    applyDensity(d);
    localStorage.setItem(DENSITY_KEY, d);
  }, []);

  const setRadius = useCallback((r: RadiusPref) => {
    setRadiusState(r);
    applyRadius(r);
    localStorage.setItem(RADIUS_KEY, r);
  }, []);

  return (
    <Ctx.Provider value={{ density, radius, setDensity, setRadius }}>{children}</Ctx.Provider>
  );
}

export function useAppearance(): AppearanceCtx {
  const c = useContext(Ctx);
  if (!c) throw new Error("useAppearance must be used within AppearanceProvider");
  return c;
}
