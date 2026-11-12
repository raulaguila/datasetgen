import "@testing-library/jest-dom/vitest";
import { vi } from "vitest";

vi.stubGlobal("matchMedia", vi.fn().mockReturnValue({
  matches: false,
  addEventListener: vi.fn(),
  removeEventListener: vi.fn(),
}));
