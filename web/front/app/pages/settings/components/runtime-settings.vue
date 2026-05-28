<script setup lang="ts">
import { api } from "~/api";
import type { SettingsEffective } from "~/api/generated/data-contracts";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();

const settings = ref<SettingsEffective[]>([]);
// Драфты — то, что в данный момент вбито в поля. Для bool/enum они не нужны
// (значения отправляются прямо из @update), но для int/url/duration/secret —
// это буфер между фокусом и кликом Save. Для secret драфт ВСЕГДА пустой,
// пока пользователь не вбил новое значение (s.value — маска, не оригинал).
const drafts = reactive<Record<string, string>>({});
const isLoading = ref(true);
const loadError = ref<string | null>(null);
const savingKey = ref<string | null>(null);

// Структура UI: фиксированный порядок групп, внутри каждой — порядок ключей.
// Метаданные (label/help) живут на клиенте, чтобы API-контракт оставался
// минимальным (`SettingsEffective` отдаёт только key/value/type/default/...).
//
// Если бэкенд зарегистрирует новую настройку, не известную этому файлу, она
// попадёт в группу "Прочее" (см. unknownGroup ниже) — UI не теряет настройки,
// просто рендерит их без человеческого label'а. Так фронт переживает мерж без
// одновременной правки META.
type Meta = { label: string; help?: string };

const META: Record<string, Meta> = {
    log_level: { label: "Log level", help: "Verbosity of the server log stream." },

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

    traffic_retention_days: {
        label: "Traffic retention",
        help: "How many days of per-device traffic counters are kept before the daily prune deletes them (1–3650).",
    },

    suggest_inspect_enabled: {
        label: "Reputation enrichment",
        help: "Run weak-lexical candidates through VirusTotal/Safe Browsing/RDAP. Requires at least one provider key below.",
    },
    virustotal_key: {
        label: "VirusTotal API key",
        help: "Key for the VT v3 endpoint. Persisted encrypted-at-rest in the local SQLite; UI shows only the last 4 characters.",
    },
    safebrowsing_key: {
        label: "Safe Browsing API key",
        help: "Google Safe Browsing v4 key. UI shows only the last 4 characters; clear with the reset button.",
    },
};

// Порядок групп жёстко зашит — у каждой свой UCard, заголовок и подсказка.
// Это и есть «настройки по фичам»: одна карточка = одна функциональная зона.
type Group = { title: string; description: string; keys: string[] };

const GROUPS: Group[] = [
    {
        title: "Logging",
        description: "Verbosity of the channel logger.",
        keys: ["log_level"],
    },
    {
        title: "Upstream DNS",
        description: "DoH endpoint and bootstrap IPs the resolver uses.",
        keys: ["doh_upstream", "doh_bootstrap_ips"],
    },
    {
        title: "Resolver cache",
        description: "Stale-while-revalidate window and refresh pool size.",
        keys: ["cache_swr", "cache_stale_grace", "cache_stale_ttl", "cache_refresh_concurrency"],
    },
    {
        title: "Traffic",
        description: "Retention of the per-device traffic counter.",
        keys: ["traffic_retention_days"],
    },
    {
        title: "Suggest-to-block · reputation inspect",
        description:
            "Optional reputation-enrichment of weak-lexical candidates. The toggle and provider keys take effect on the next inspect tick; no restart is required.",
        keys: ["suggest_inspect_enabled", "virustotal_key", "safebrowsing_key"],
    },
];

const labelFor = (s: SettingsEffective) => META[s.key ?? ""]?.label ?? s.key ?? "";
const helpFor = (s: SettingsEffective) => META[s.key ?? ""]?.help;

// Раскладываем загруженные настройки по группам. Незнакомые (бэкенд впереди
// фронта) попадают в отдельную секцию "Other" в самом конце, чтобы оператор
// мог их хотя бы увидеть и поменять до следующего релиза UI.
const grouped = computed(() => {
    const byKey = new Map<string, SettingsEffective>();
    for (const s of settings.value) {
        if (s.key) byKey.set(s.key, s);
    }
    const seen = new Set<string>();
    const out: Array<Group & { items: SettingsEffective[] }> = [];
    for (const g of GROUPS) {
        const items = g.keys
            .map((k) => byKey.get(k))
            .filter((s): s is SettingsEffective => Boolean(s));
        items.forEach((s) => s.key && seen.add(s.key));
        if (items.length > 0) out.push({ ...g, items });
    }
    const unknown = settings.value.filter((s) => s.key && !seen.has(s.key));
    if (unknown.length > 0) {
        out.push({
            title: "Other",
            description: "Settings the UI doesn't yet know about — please update the client.",
            keys: [],
            items: unknown,
        });
    }
    return out;
});

const load = async () => {
    isLoading.value = true;
    loadError.value = null;
    try {
        const list = await api.listSettings();
        settings.value = list;
        for (const s of list) {
            if (!s.key) continue;
            // Для секретов значение, приехавшее с бэка, — это маска ("••••abcd").
            // Класть её в drafts нельзя, иначе пользователь нажмёт "Save" и
            // отправит маску. Драфт остаётся пустым, поле — password,
            // placeholder покажет маску для верификации, что записано.
            drafts[s.key] = s.type === "secret" ? "" : (s.value ?? "");
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

// draftString нормализует значение драфта в строку перед сравнением/отправкой.
// Nuxt UI UInput с type='number' через looseToNumber() coerce-ит v-model в
// JavaScript number; типизированный API ожидает строку, и Go-бэкенд биндит
// JSON-number в Go-string-поле как 400. String(x ?? '') одинаково работает и
// для number ("5"), и для string ("5"), и для undefined ("") — единая форма.
const draftString = (key: string) => String(drafts[key] ?? "");

// "Грязно" для текстовых полей = драфт (приведённый к строке) отличается от
// значения с бэка. Для секретов значение с бэка — маска, поэтому считаем
// грязным любой непустой непробельный драфт (пустой/пробелы = нечего
// сохранять, используйте Reset для очистки).
const isDirty = (s: SettingsEffective) => {
    if (!s.key) return false;
    if (s.type === "secret") return draftString(s.key).trim() !== "";
    return draftString(s.key) !== (s.value ?? "");
};

onMounted(load);
</script>

<template>
    <div class="space-y-6">
        <!-- Visible error state: never sit in a permanent skeleton if the API failed. -->
        <UAlert
            v-if="loadError"
            color="error"
            variant="subtle"
            icon="i-lucide-triangle-alert"
            title="Could not load settings"
            :description="loadError"
            :actions="[
                {
                    label: 'Retry',
                    icon: 'i-lucide-refresh-cw',
                    color: 'neutral',
                    variant: 'subtle',
                    onClick: load,
                },
            ]"
        />

        <UCard v-else-if="isLoading">
            <div class="space-y-3">
                <USkeleton v-for="n in 4" :key="n" class="h-10 w-full" />
            </div>
        </UCard>

        <UCard v-for="group in grouped" v-else :key="group.title">
            <template #header>
                <div class="space-y-1">
                    <h2 class="text-base font-semibold text-highlighted">{{ group.title }}</h2>
                    <p class="text-sm text-muted">{{ group.description }}</p>
                </div>
            </template>

            <div class="divide-y divide-default">
                <div
                    v-for="s in group.items"
                    :key="s.key"
                    class="flex flex-col gap-2 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-start sm:justify-between"
                >
                    <div class="space-y-0.5 sm:max-w-xs">
                        <div class="flex items-center gap-2">
                            <span class="text-sm font-medium text-highlighted">
                                {{ labelFor(s) }}
                            </span>
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

                        <!-- int → number input + explicit Save. UInput coerce-ит v-model в number; -->
                        <!-- save отправляет уже String(draft), бэкенд ждёт строку.            -->
                        <template v-else-if="s.type === 'int'">
                            <UInput
                                v-if="s.key"
                                v-model="drafts[s.key]"
                                type="number"
                                :min="1"
                                :disabled="savingKey === s.key"
                                class="w-56"
                                @keyup.enter="save(s, draftString(s.key))"
                            />
                            <UButton
                                color="primary"
                                variant="solid"
                                :loading="savingKey === s.key"
                                :disabled="!isDirty(s)"
                                @click="save(s, s.key ? draftString(s.key) : '')"
                            >
                                Save
                            </UButton>
                        </template>

                        <!-- secret → password input + explicit Save; маска видна как placeholder -->
                        <template v-else-if="s.type === 'secret'">
                            <UInput
                                v-if="s.key"
                                v-model="drafts[s.key]"
                                type="password"
                                autocomplete="off"
                                :placeholder="s.value || 'not set'"
                                :disabled="savingKey === s.key"
                                class="w-56"
                                @keyup.enter="save(s, draftString(s.key).trim())"
                            />
                            <UButton
                                color="primary"
                                variant="solid"
                                :loading="savingKey === s.key"
                                :disabled="!isDirty(s)"
                                @click="save(s, s.key ? draftString(s.key).trim() : '')"
                            >
                                Save
                            </UButton>
                        </template>

                        <!-- everything else (url, ip-list, duration, …) → text input + Save -->
                        <template v-else>
                            <UInput
                                v-if="s.key"
                                v-model="drafts[s.key]"
                                :disabled="savingKey === s.key"
                                class="w-56"
                                @keyup.enter="save(s, draftString(s.key))"
                            />
                            <UButton
                                color="primary"
                                variant="solid"
                                :loading="savingKey === s.key"
                                :disabled="!isDirty(s)"
                                @click="save(s, s.key ? draftString(s.key) : '')"
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
                            :title="
                                s.type === 'secret'
                                    ? 'Clear override (reset to env default)'
                                    : `Reset to default (${s.default})`
                            "
                            @click="reset(s)"
                        />
                    </div>
                </div>
            </div>
        </UCard>
    </div>
</template>
