<script setup lang="ts">
import { api } from "~/api";
import type { SettingsEffective } from "~/api/generated/data-contracts";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();

const settings = ref<SettingsEffective[]>([]);
const drafts = reactive<Record<string, string>>({});
const isLoading = ref(true);
const loadError = ref<string | null>(null);
const savingKey = ref<string | null>(null);

// Presentation metadata lives on the client so the API contract stays minimal
// (the backend only reports value/type/enum/default/overridden). Unknown keys
// fall back to the raw key.
const META: Record<string, { label: string; help?: string }> = {
    log_level: {
        label: "Log level",
        help: "Verbosity of the server log stream.",
    },
    doh_upstream: {
        label: "DoH upstream",
        help: "Endpoint DNS queries are forwarded to over HTTPS.",
    },
    doh_bootstrap_ips: {
        label: "DoH bootstrap IPs",
        help: "Comma-separated IPs used to reach the DoH host without relying on system DNS.",
    },
    cache_swr: {
        label: "Stale-while-revalidate",
        help: "Serve a stale answer instantly on a TTL boundary and refresh in the background.",
    },
    cache_stale_grace: {
        label: "Stale grace window",
        help: "How long past its TTL a cached answer may still be served as stale (e.g. 24h).",
    },
    cache_stale_ttl: {
        label: "Stale TTL",
        help: "TTL written to a stale response so clients come back soon (e.g. 30s).",
    },
    cache_refresh_concurrency: {
        label: "Refresh concurrency",
        help: "Maximum number of background refreshes allowed in flight.",
    },
};

const labelFor = (s: SettingsEffective) => META[s.key ?? ""]?.label ?? s.key ?? "";
const helpFor = (s: SettingsEffective) => META[s.key ?? ""]?.help;

const load = async () => {
    isLoading.value = true;
    loadError.value = null;
    try {
        const list = await api.listSettings();
        settings.value = list;
        for (const s of list) {
            if (s.key) drafts[s.key] = s.value ?? "";
        }
    } catch (error) {
        loadError.value = getErrorMessage(error);
    } finally {
        isLoading.value = false;
    }
};

const save = async (s: SettingsEffective, value: string) => {
    if (!s.key) return;
    savingKey.value = s.key;
    try {
        await api.updateSetting(s.key, value);
        toast.add({ title: "Saved", description: `${labelFor(s)} updated.`, duration: 3000 });
        await load();
    } catch (error) {
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
        await load(); // pull authoritative values back, discarding the failed edit
    } finally {
        savingKey.value = null;
    }
};

const reset = async (s: SettingsEffective) => {
    if (!s.key) return;
    savingKey.value = s.key;
    try {
        await api.resetSetting(s.key);
        toast.add({
            title: "Reset",
            description: `${labelFor(s)} reverted to its default.`,
            duration: 3000,
        });
        await load();
    } catch (error) {
        toast.add({
            title: "Error",
            description: getErrorMessage(error),
            duration: 5000,
            color: "error",
        });
    } finally {
        savingKey.value = null;
    }
};

const boolValue = (s: SettingsEffective) => (s.value ?? "false").toLowerCase() === "true";
const isDirty = (s: SettingsEffective) => s.key !== undefined && drafts[s.key] !== (s.value ?? "");

onMounted(load);
</script>

<template>
    <UCard>
        <template #header>
            <div class="flex items-start justify-between gap-4">
                <div class="space-y-1">
                    <h2 class="text-base font-semibold text-highlighted">Resolver &amp; runtime</h2>
                    <p class="text-sm text-muted">
                        Persisted in the database — changes apply immediately and survive a restart.
                    </p>
                </div>
                <UButton
                    v-if="loadError"
                    icon="i-lucide-refresh-cw"
                    color="neutral"
                    variant="subtle"
                    :loading="isLoading"
                    @click="load"
                >
                    Retry
                </UButton>
            </div>
        </template>

        <!-- Visible error state: never sit in a permanent skeleton if the API failed. -->
        <UAlert
            v-if="loadError"
            color="error"
            variant="subtle"
            icon="i-lucide-triangle-alert"
            title="Could not load settings"
            :description="loadError"
        />

        <div v-else-if="isLoading" class="space-y-4">
            <USkeleton v-for="n in 4" :key="n" class="h-10 w-full" />
        </div>

        <div v-else class="divide-y divide-default">
            <div
                v-for="s in settings"
                :key="s.key"
                class="flex flex-col gap-2 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-start sm:justify-between"
            >
                <div class="space-y-0.5 sm:max-w-xs">
                    <div class="flex items-center gap-2">
                        <span class="text-sm font-medium text-highlighted">{{ labelFor(s) }}</span>
                        <UBadge v-if="s.overridden" color="primary" variant="subtle" size="sm">
                            custom
                        </UBadge>
                    </div>
                    <p v-if="helpFor(s)" class="text-xs text-muted">{{ helpFor(s) }}</p>
                </div>

                <div class="flex items-center gap-2">
                    <!-- boolean → switch, saves on toggle -->
                    <USwitch
                        v-if="s.type === 'bool'"
                        :model-value="boolValue(s)"
                        :loading="savingKey === s.key"
                        :disabled="savingKey === s.key"
                        @update:model-value="(v: boolean) => save(s, v ? 'true' : 'false')"
                    />

                    <!-- enum → select, saves on change -->
                    <USelect
                        v-else-if="s.type === 'enum'"
                        :model-value="s.value"
                        :items="s.enum ?? []"
                        :loading="savingKey === s.key"
                        :disabled="savingKey === s.key"
                        class="w-56"
                        @update:model-value="(v: string) => save(s, v)"
                    />

                    <!-- everything else → text input + explicit Save -->
                    <template v-else>
                        <UInput
                            v-if="s.key"
                            v-model="drafts[s.key]"
                            :disabled="savingKey === s.key"
                            class="w-56"
                            @keyup.enter="save(s, drafts[s.key] ?? '')"
                        />
                        <UButton
                            color="primary"
                            variant="solid"
                            :loading="savingKey === s.key"
                            :disabled="!isDirty(s)"
                            @click="save(s, s.key ? (drafts[s.key] ?? '') : '')"
                        >
                            Save
                        </UButton>
                    </template>

                    <UButton
                        v-if="s.overridden"
                        icon="i-lucide-rotate-ccw"
                        color="neutral"
                        variant="ghost"
                        :loading="savingKey === s.key"
                        :title="`Reset to default (${s.default})`"
                        @click="reset(s)"
                    />
                </div>
            </div>
        </div>
    </UCard>
</template>
