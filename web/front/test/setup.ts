import { vi } from "vitest";

// Stub the Nuxt UI auto-imported `useToast` so composables that call it work
// without booting Nuxt. Tests that need to assert toasts can re-stub via
// `vi.stubGlobal("useToast", () => ...)` in their own setup.
const defaultToast = { add: vi.fn(), remove: vi.fn(), update: vi.fn(), clear: vi.fn() };
vi.stubGlobal("useToast", () => defaultToast);
