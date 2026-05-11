import { reactive } from "vue";
import { api } from "~/api";
import type { DomainInspectInspectResult } from "~/api/generated/data-contracts";
import { getErrorMessage } from "../utils/get-error-message";
import { isAbortError } from "../utils/is-abort-error";

export type InspectStatus = "idle" | "loading" | "done" | "error";

export interface InspectEntry {
    status: InspectStatus;
    result: DomainInspectInspectResult | null;
    error: string;
}

// Module-level singleton: scan results are cached for the whole SPA session so
// the suggest table doesn't re-hammer the upstream APIs (Google Safe Browsing,
// VirusTotal, ...) on every pagination / re-render, and so the inspect drawer
// renders instantly when the background scan has already finished.
const MAX_CONCURRENT = 2;
const registry = reactive(new Map<string, InspectEntry>());
const queue: string[] = [];
const controllers = new Map<string, AbortController>();
let active = 0;

const normalize = (domain: string): string => domain.trim().toLowerCase();

const ensure = (key: string): InspectEntry => {
    let entry = registry.get(key);
    if (!entry) {
        entry = { status: "idle", result: null, error: "" };
        registry.set(key, entry);
    }
    return entry;
};

const pump = () => {
    while (active < MAX_CONCURRENT && queue.length > 0) {
        const next = queue.shift()!;
        void runOne(next);
    }
};

const runOne = async (key: string) => {
    const entry = ensure(key);
    if (entry.status !== "idle") return;
    active++;
    entry.status = "loading";
    entry.error = "";
    const controller = new AbortController();
    controllers.set(key, controller);
    try {
        entry.result = await api.inspectDomain(key, controller.signal);
        entry.status = "done";
    } catch (e) {
        if (isAbortError(e)) {
            // Aborted — leave the entry retryable so a later register() picks
            // it up again instead of being stuck in a permanent "error" state.
            entry.status = "idle";
        } else {
            entry.error = getErrorMessage(e);
            entry.status = "error";
        }
    } finally {
        controllers.delete(key);
        active--;
        pump();
    }
};

export const useInspectRegistry = () => {
    const register = (domain: string) => {
        const key = normalize(domain);
        if (!key) return;
        const entry = ensure(key);
        if (entry.status === "idle" && !queue.includes(key) && !controllers.has(key)) {
            queue.push(key);
            pump();
        }
    };

    const get = (domain: string): InspectEntry | undefined => {
        const key = normalize(domain);
        if (!key) return undefined;
        return registry.get(key);
    };

    return { register, get };
};

// Test-only — resets the module-level cache so unit tests don't leak state.
export const __clearInspectRegistry = () => {
    controllers.forEach((c) => c.abort());
    controllers.clear();
    queue.length = 0;
    registry.clear();
    active = 0;
};
