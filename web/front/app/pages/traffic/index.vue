<script setup lang="ts">
import type { WebDeviceDTO } from "~/api/generated/data-contracts";
import { useTrafficDashboard, type VerdictFilter } from "~~/composables/use-traffic-dashboard";
import { formatDate } from "~~/utils/format-date";

useHead({
    title: "Traffic",
    link: [
        { rel: "preconnect", href: "https://fonts.googleapis.com" },
        { rel: "preconnect", href: "https://fonts.gstatic.com", crossorigin: "" },
        {
            rel: "stylesheet",
            href: "https://fonts.googleapis.com/css2?family=Instrument+Serif:ital@0;1&family=JetBrains+Mono:wght@300;400;500&display=swap",
        },
    ],
});

const t = useTrafficDashboard();

// One verdict filter drives the headline number AND the ranked list, so the big
// metric, the "top targets" panel and the segmented control all read as a single
// coherent view.
const verdictOptions: { label: string; value: VerdictFilter }[] = [
    { label: "All", value: "all" },
    { label: "Blocked", value: "blocked" },
    { label: "Allowed", value: "allowed" },
];

const TOP_DISPLAY = 15; // ranked rows shown with bars

// Two views over the same data: the ranked "top targets" list and the
// per-device breakdown. The headline + verdict filter live above the tabs, so
// the big number stays in view while switching.
type TrafficTab = "top" | "devices";
const activeTab = ref<TrafficTab>("top");
const tabs: { label: string; value: TrafficTab; icon: string }[] = [
    { label: "Top domains", value: "top", icon: "i-lucide-trophy" },
    { label: "Devices", value: "devices", icon: "i-lucide-monitor-smartphone" },
];

const formatNumber = (n: number | undefined) => (n ?? 0).toLocaleString("en-US");

// Gates the hero-art intro fade-in: false on first paint, flipped on mount.
const ready = ref(false);

// --- Headline metric: animated count-up that re-runs whenever the verdict
// filter (and thus heroMetric) changes. Each run cancels the previous frame
// loop so rapid filter toggles can't leave two loops racing on displayMetric. ---
const displayMetric = ref(0);
let animateRaf: number | null = null;
const animate = (target: number, duration = 1300) => {
    if (animateRaf !== null) cancelAnimationFrame(animateRaf);
    const start = performance.now();
    const from = displayMetric.value;
    const tick = (now: number) => {
        const p = Math.min((now - start) / duration, 1);
        const eased = p === 1 ? 1 : 1 - Math.pow(2, -10 * p);
        displayMetric.value = Math.floor(from + (target - from) * eased);
        animateRaf = p < 1 ? requestAnimationFrame(tick) : null;
    };
    animateRaf = requestAnimationFrame(tick);
};
watch(
    () => t.heroMetric.value,
    (v) => animate(v),
);

const heroEyebrow = computed(() => {
    switch (t.topBlockedFilter.value) {
        case "blocked":
            return "Blocked ledger";
        case "allowed":
            return "Allowed ledger";
        default:
            return "Traffic ledger";
    }
});

const heroLabel = computed(() => {
    switch (t.topBlockedFilter.value) {
        case "blocked":
            return "queries intercepted & resolved as NXDOMAIN at the sinkhole";
        case "allowed":
            return "queries resolved and forwarded upstream over DoH";
        default:
            return "DNS queries observed across every device on the network";
    }
});

// --- Ranked "top targets" list ---
const topRanked = computed(() => t.topDomains.value.slice(0, TOP_DISPLAY));
const topMax = computed(() => Math.max(...topRanked.value.map((d) => d.count ?? 0), 1));

// --- Devices ---
// Title precedence: the friendly mDNS hostname is the most human-readable, then
// the OUI vendor, then the raw IP. The MAC/IP identifier always stays visible in
// the subtitle, so promoting a name here never hides which device it is.
const deviceTitle = (d: WebDeviceDTO) => {
    if (d.hostname) return d.hostname;
    if (d.client_kind === "mac" && d.vendor) return d.vendor;
    return d.current_ip || d.client_value || "Unknown device";
};
const deviceSubtitle = (d: WebDeviceDTO) => {
    const parts = [d.client_value ?? "—"];
    if (d.client_kind === "mac" && d.current_ip) parts.push(d.current_ip);
    return parts.join("  ·  ");
};

const onSelectDevice = (device: WebDeviceDTO) => {
    void t.selectDevice(device);
};

// --- Device drill-down side panel (UDrawer, slides from the right) ---
const drawerOpen = computed({
    get: () => t.selectedDevice.value !== null,
    set: (v: boolean) => {
        if (!v) t.clearSelection();
    },
});
const drawerMax = computed(() => Math.max(...t.domains.value.map((d) => d.count ?? 0), 1));

// Re-fetch the ranked list when the shared verdict filter changes (the headline
// metric updates reactively, no fetch needed).
watch(t.topBlockedFilter, () => t.loadTopDomains());

// Re-fetch the drill-down when its own verdict filter changes (resets to page 1).
watch(t.blockedFilter, () => {
    if (t.selectedDevice.value) void t.reloadDomainsFromStart();
});

onMounted(() => {
    ready.value = true;
    void t.loadDevices();
    void t.loadTopDomains();
});
</script>

<template>
    <div class="ops" :data-verdict="t.topBlockedFilter.value">
        <div class="ops__grid" aria-hidden="true"></div>

        <UContainer class="ops__inner">
            <!-- ============ HERO ============ -->
            <section class="ops__hero">
                <div class="ops__hero-text">
                    <p class="ops__eyebrow">
                        <span>{{ heroEyebrow }}</span>
                        <span class="ops__eyebrow-dot"></span>
                        <span>since uptime</span>
                    </p>

                    <h1 class="ops__metric">
                        <span class="ops__metric-value">{{ formatNumber(displayMetric) }}</span>
                    </h1>

                    <p class="ops__metric-label">{{ heroLabel }}</p>

                    <div
                        class="ops__seg ops__seg--hero"
                        role="radiogroup"
                        aria-label="Verdict filter"
                    >
                        <button
                            v-for="o in verdictOptions"
                            :key="o.value"
                            type="button"
                            role="radio"
                            :aria-checked="t.topBlockedFilter.value === o.value"
                            class="ops__seg-btn"
                            :class="{
                                'ops__seg-btn--active': t.topBlockedFilter.value === o.value,
                            }"
                            @click="t.topBlockedFilter.value = o.value"
                        >
                            {{ o.label }}
                        </button>
                    </div>

                    <div class="ops__rule"></div>
                </div>

                <div class="ops__hero-art" :class="{ 'ops__hero-art--ready': ready }">
                    <svg viewBox="0 0 240 240" aria-hidden="true">
                        <defs>
                            <radialGradient id="ringGlow" cx="50%" cy="50%" r="50%">
                                <stop offset="0%" stop-color="currentColor" stop-opacity="0.25" />
                                <stop offset="60%" stop-color="currentColor" stop-opacity="0" />
                            </radialGradient>
                        </defs>
                        <circle cx="120" cy="120" r="118" fill="url(#ringGlow)" />
                        <g class="ops__hero-art-spin">
                            <circle
                                cx="120"
                                cy="120"
                                r="100"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="0.4"
                                stroke-dasharray="1 5"
                            />
                            <circle
                                cx="120"
                                cy="120"
                                r="80"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="0.4"
                            />
                            <circle
                                cx="120"
                                cy="120"
                                r="60"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="0.4"
                                stroke-dasharray="3 3"
                            />
                        </g>
                        <g class="ops__hero-art-counter">
                            <circle
                                cx="120"
                                cy="120"
                                r="40"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="0.4"
                            />
                            <circle cx="120" cy="120" r="2" fill="currentColor" />
                        </g>
                        <line
                            x1="120"
                            y1="2"
                            x2="120"
                            y2="238"
                            stroke="currentColor"
                            stroke-width="0.3"
                            stroke-dasharray="2 6"
                        />
                        <line
                            x1="2"
                            y1="120"
                            x2="238"
                            y2="120"
                            stroke="currentColor"
                            stroke-width="0.3"
                            stroke-dasharray="2 6"
                        />
                    </svg>
                </div>
            </section>

            <!-- ============ TAB BAR ============ -->
            <div class="ops__tabs" role="tablist" aria-label="Traffic view">
                <button
                    v-for="tab in tabs"
                    :id="`tab-${tab.value}`"
                    :key="tab.value"
                    type="button"
                    class="ops__tab"
                    :class="{ 'ops__tab--active': activeTab === tab.value }"
                    role="tab"
                    :aria-selected="activeTab === tab.value"
                    :aria-controls="`panel-${tab.value}`"
                    @click="activeTab = tab.value"
                >
                    <UIcon :name="tab.icon" class="ops__tab-icon" />
                    {{ tab.label }}
                </button>
            </div>

            <!-- ============ TOP TARGETS ============ -->
            <section
                v-if="activeTab === 'top'"
                id="panel-top"
                class="ops__panel"
                role="tabpanel"
                aria-labelledby="tab-top"
            >
                <div class="ops__panel-corner ops__panel-corner--tl"></div>
                <div class="ops__panel-corner ops__panel-corner--tr"></div>
                <div class="ops__panel-corner ops__panel-corner--bl"></div>
                <div class="ops__panel-corner ops__panel-corner--br"></div>

                <header class="ops__panel-head">
                    <div>
                        <h2><em>Top targets</em></h2>
                        <p>most-queried domains · ranked top {{ TOP_DISPLAY }}</p>
                    </div>
                    <span class="ops__panel-axis">{{ heroEyebrow }}</span>
                </header>

                <UAlert
                    v-if="t.topDomainsError.value"
                    color="error"
                    variant="subtle"
                    icon="i-lucide-circle-x"
                    title="Failed to load top targets"
                    :description="t.topDomainsError.value"
                    :actions="[
                        {
                            label: 'Retry',
                            color: 'neutral',
                            variant: 'outline',
                            onClick: () => t.loadTopDomains(),
                        },
                    ]"
                />
                <div v-else-if="t.topDomainsLoading.value" class="ops__skeleton">
                    <span v-for="n in 6" :key="n"></span>
                </div>
                <ol v-else-if="topRanked.length" class="ops__list">
                    <li
                        v-for="(g, i) in topRanked"
                        :key="g.domain ?? i"
                        class="ops__row"
                        :style="{ '--i': i, '--w': `${((g.count ?? 0) / topMax) * 100}%` }"
                    >
                        <span class="ops__rank">{{ String(i + 1).padStart(2, "0") }}</span>
                        <span class="ops__domain" :title="g.domain ?? ''">{{ g.domain }}</span>
                        <div class="ops__bar"><div class="ops__bar-fill"></div></div>
                        <span class="ops__count">{{ formatNumber(g.count ?? 0) }}</span>
                    </li>
                </ol>
                <p v-else class="ops__empty">no signal · ledger empty</p>
            </section>

            <!-- ============ DEVICES ============ -->
            <section
                v-else
                id="panel-devices"
                class="ops__panel"
                role="tabpanel"
                aria-labelledby="tab-devices"
            >
                <div class="ops__panel-corner ops__panel-corner--tl"></div>
                <div class="ops__panel-corner ops__panel-corner--tr"></div>
                <div class="ops__panel-corner ops__panel-corner--bl"></div>
                <div class="ops__panel-corner ops__panel-corner--br"></div>

                <header class="ops__panel-head">
                    <div>
                        <h2><em>Devices</em></h2>
                        <p>per-device traffic · {{ t.deviceCount.value }} seen</p>
                    </div>
                    <span class="ops__panel-axis">tap a row to inspect its domains →</span>
                </header>

                <UAlert
                    v-if="t.devicesError.value"
                    color="error"
                    variant="subtle"
                    icon="i-lucide-circle-x"
                    title="Failed to load devices"
                    :description="t.devicesError.value"
                    :actions="[
                        {
                            label: 'Retry',
                            color: 'neutral',
                            variant: 'outline',
                            onClick: () => t.loadDevices(),
                        },
                    ]"
                />
                <div v-else-if="t.devicesLoading.value" class="ops__skeleton">
                    <span v-for="n in 5" :key="n"></span>
                </div>
                <ul v-else-if="t.devices.value.length" class="ops__devs">
                    <li
                        v-for="d in t.devices.value"
                        :key="`${d.client_kind}:${d.client_value}`"
                        class="ops__dev"
                        :class="{ 'ops__dev--active': t.selectedKey.value === t.deviceKey(d) }"
                        role="button"
                        tabindex="0"
                        @click="onSelectDevice(d)"
                        @keydown.enter.prevent="onSelectDevice(d)"
                        @keydown.space.prevent="onSelectDevice(d)"
                    >
                        <span
                            class="ops__dev-kind"
                            :class="d.client_kind === 'mac' ? 'is-mac' : 'is-ip'"
                        >
                            {{ d.client_kind === "mac" ? "MAC" : "IP" }}
                        </span>
                        <span class="ops__dev-id">
                            <span class="ops__dev-title">{{ deviceTitle(d) }}</span>
                            <span class="ops__dev-sub">{{ deviceSubtitle(d) }}</span>
                        </span>
                        <span class="ops__dev-stat ops__dev-stat--good">
                            <em>allowed</em>{{ formatNumber(d.allowed_count) }}
                        </span>
                        <span class="ops__dev-stat ops__dev-stat--bad">
                            <em>blocked</em>{{ formatNumber(d.blocked_count) }}
                        </span>
                        <span class="ops__dev-seen">
                            <em>last seen</em>{{ d.last_seen ? formatDate(d.last_seen) : "—" }}
                        </span>
                        <UIcon name="i-lucide-chevron-right" class="ops__dev-arrow" />
                    </li>
                </ul>
                <p v-else class="ops__empty">no devices observed yet</p>
            </section>
        </UContainer>

        <!-- ============ DRILL-DOWN SIDE PANEL ============ -->
        <UDrawer v-model:open="drawerOpen" direction="right" :handle="false">
            <template #header>
                <div class="flex flex-col gap-0.5 min-w-0">
                    <span class="text-xs uppercase tracking-wider text-muted">Domains for</span>
                    <span class="font-medium truncate">{{
                        t.selectedDevice.value ? deviceTitle(t.selectedDevice.value) : ""
                    }}</span>
                    <span class="font-mono text-xs text-muted truncate">{{
                        t.selectedDevice.value?.client_value
                    }}</span>
                </div>
            </template>

            <template #body>
                <div class="ops-drawer">
                    <div
                        class="ops__seg ops__seg--drawer"
                        role="radiogroup"
                        aria-label="Verdict filter"
                    >
                        <button
                            v-for="o in verdictOptions"
                            :key="o.value"
                            type="button"
                            role="radio"
                            :aria-checked="t.blockedFilter.value === o.value"
                            class="ops__seg-btn"
                            :class="{ 'ops__seg-btn--active': t.blockedFilter.value === o.value }"
                            @click="t.blockedFilter.value = o.value"
                        >
                            {{ o.label }}
                        </button>
                    </div>

                    <UAlert
                        v-if="t.domainsError.value"
                        color="error"
                        variant="subtle"
                        icon="i-lucide-circle-x"
                        title="Failed to load domains"
                        :description="t.domainsError.value"
                        class="mt-4"
                        :actions="[
                            {
                                label: 'Retry',
                                color: 'neutral',
                                variant: 'outline',
                                onClick: () => t.loadDomains(),
                            },
                        ]"
                    />
                    <div v-else-if="t.domainsLoading.value" class="ops__skeleton mt-4">
                        <span v-for="n in 6" :key="n"></span>
                    </div>
                    <template v-else-if="t.domains.value.length">
                        <ol class="ops__list ops__list--drawer">
                            <li
                                v-for="(g, i) in t.domains.value"
                                :key="g.domain ?? i"
                                class="ops__row"
                                :style="{
                                    '--i': i,
                                    '--w': `${((g.count ?? 0) / drawerMax) * 100}%`,
                                }"
                            >
                                <span class="ops__domain" :title="g.domain ?? ''">{{
                                    g.domain
                                }}</span>
                                <div class="ops__bar"><div class="ops__bar-fill"></div></div>
                                <span class="ops__count">{{ formatNumber(g.count ?? 0) }}</span>
                            </li>
                        </ol>
                        <div
                            v-if="t.domainsTotal.value > t.domainsPageSize"
                            class="flex justify-center pt-4 mt-2 border-t border-default"
                        >
                            <UPagination
                                :page="t.domainsPageIndex.value + 1"
                                :items-per-page="t.domainsPageSize"
                                :total="t.domainsTotal.value"
                                @update:page="(p: number) => t.changeDomainsPage(p)"
                            />
                        </div>
                    </template>
                    <p v-else class="ops__empty">no domains for this device</p>
                </div>
            </template>
        </UDrawer>
    </div>
</template>

<style scoped>
.ops {
    --bg: var(--ui-bg);
    --bg-elev: var(--ui-bg-elevated);
    --line: var(--ui-border);
    --line-soft: var(--ui-border-muted);
    --text: var(--ui-text-highlighted);
    --body: var(--ui-text);
    --muted: var(--ui-text-muted);
    --dim: var(--ui-text-dimmed);
    --accent: var(--ui-color-error-500);
    --good: var(--ui-color-primary-500);

    /* The active verdict tints the bars + hero art. */
    --verdict: var(--accent);

    min-height: calc(100vh - var(--ui-header-height));
    background:
        radial-gradient(
            1200px 600px at 75% -10%,
            color-mix(in srgb, var(--verdict) 8%, transparent),
            transparent 60%
        ),
        radial-gradient(
            900px 500px at 0% 110%,
            color-mix(in srgb, var(--good) 6%, transparent),
            transparent 55%
        ),
        var(--bg);
    color: var(--body);
    font-family: "Instrument Serif", "Times New Roman", Georgia, serif;
    padding: clamp(1.25rem, 3vw, 2rem) 0;
    position: relative;
    overflow: hidden;
    display: flex;
    transition: background 0.5s ease;
}

.ops[data-verdict="allowed"] {
    --verdict: var(--good);
}
.ops[data-verdict="all"] {
    --verdict: color-mix(in srgb, var(--accent), var(--good));
}

.ops__inner {
    display: grid;
    grid-template-rows: auto auto auto auto;
    gap: clamp(1.25rem, 2.5vw, 2rem);
    width: 100%;
    position: relative;
    z-index: 1;
}

.ops__grid {
    position: absolute;
    inset: 0;
    background-image:
        linear-gradient(color-mix(in srgb, var(--text) 4%, transparent) 1px, transparent 1px),
        linear-gradient(90deg, color-mix(in srgb, var(--text) 4%, transparent) 1px, transparent 1px);
    background-size: 64px 64px;
    pointer-events: none;
    mask-image: radial-gradient(ellipse at center, black 30%, transparent 80%);
}

.ops > *:not(.ops__grid) {
    position: relative;
    z-index: 1;
}

/* HERO */
.ops__hero {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: clamp(1.5rem, 4vw, 3rem);
    padding: clamp(0.5rem, 2vw, 1.5rem) 0 0;
}

.ops__eyebrow {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.7rem;
    letter-spacing: 0.22em;
    text-transform: uppercase;
    color: var(--muted);
    margin: 0 0 0.5rem;
}

.ops__eyebrow-dot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--dim);
}

.ops__metric {
    font-family: "JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-weight: 500;
    font-size: clamp(3.5rem, 11vw, 9rem);
    line-height: 0.9;
    letter-spacing: -0.07em;
    margin: 0;
}

.ops__metric-value {
    display: inline-block;
    font-variant-numeric: tabular-nums;
    color: var(--text);
}

.ops__metric-label {
    font-size: clamp(1.05rem, 1.5vw, 1.3rem);
    color: var(--muted);
    line-height: 1.45;
    max-width: 40ch;
    margin: 1.25rem 0 0;
}

.ops__rule {
    width: clamp(140px, 18vw, 240px);
    height: 1px;
    background: linear-gradient(90deg, var(--verdict), var(--line) 70%, transparent);
    margin: 1.5rem 0 1rem;
    transition: background 0.5s ease;
}

.ops__hero-art {
    position: relative;
    width: clamp(180px, 18vw, 280px);
    aspect-ratio: 1;
    color: var(--verdict);
    opacity: 0;
    transition:
        opacity 0.8s 0.3s ease,
        color 0.5s ease;
}

.ops__hero-art--ready {
    opacity: 1;
}

.ops__hero-art svg {
    width: 100%;
    height: 100%;
    display: block;
}

.ops__hero-art-spin {
    transform-origin: 120px 120px;
    animation: spin 80s linear infinite;
}

.ops__hero-art-counter {
    transform-origin: 120px 120px;
    animation: spin 40s linear infinite reverse;
}

@keyframes spin {
    to {
        transform: rotate(360deg);
    }
}

/* PANEL */
.ops__panel {
    background: linear-gradient(
        180deg,
        var(--bg-elev),
        color-mix(in srgb, var(--bg-elev), var(--bg) 30%)
    );
    border: 1px solid var(--line);
    padding: clamp(1.25rem, 2.5vw, 2rem) clamp(1.25rem, 3vw, 2.5rem);
    position: relative;
}

.ops__panel-corner {
    position: absolute;
    width: 10px;
    height: 10px;
    border: 1px solid var(--verdict);
}
.ops__panel-corner--tl {
    top: -1px;
    left: -1px;
    border-right: none;
    border-bottom: none;
}
.ops__panel-corner--tr {
    top: -1px;
    right: -1px;
    border-left: none;
    border-bottom: none;
}
.ops__panel-corner--bl {
    bottom: -1px;
    left: -1px;
    border-right: none;
    border-top: none;
}
.ops__panel-corner--br {
    bottom: -1px;
    right: -1px;
    border-left: none;
    border-top: none;
}

.ops__panel-head {
    display: flex;
    justify-content: space-between;
    align-items: end;
    gap: 1rem;
    border-bottom: 1px solid var(--line);
    padding-bottom: 1rem;
    margin-bottom: 1.25rem;
    flex-wrap: wrap;
}

.ops__panel-head h2 {
    font-family: "Instrument Serif", serif;
    font-weight: 400;
    font-size: clamp(1.4rem, 2.2vw, 1.875rem);
    color: var(--text);
    margin: 0;
    line-height: 1;
}

.ops__panel-head h2 em {
    font-style: italic;
}

.ops__panel-head p {
    margin: 0.25rem 0 0;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.65rem;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    color: var(--muted);
}

.ops__panel-axis {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.62rem;
    letter-spacing: 0.1em;
    color: var(--dim);
    white-space: nowrap;
}

/* SEGMENTED VERDICT CONTROL */
.ops__seg {
    display: inline-flex;
    border: 1px solid var(--line);
    background: var(--bg);
}

.ops__seg--drawer {
    width: 100%;
}

.ops__seg--hero {
    margin-top: 1.5rem;
}

.ops__seg-btn {
    flex: 1;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.66rem;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--muted);
    background: transparent;
    border: none;
    padding: 0.45rem 0.9rem;
    cursor: pointer;
    transition:
        color 0.2s ease,
        background 0.2s ease;
}

.ops__seg-btn + .ops__seg-btn {
    border-left: 1px solid var(--line);
}

.ops__seg-btn:hover {
    color: var(--text);
}

.ops__seg-btn--active {
    color: var(--text);
    background: color-mix(in srgb, var(--verdict) 16%, transparent);
}

/* TAB BAR (Top targets / Devices) */
.ops__tabs {
    display: flex;
    gap: 0.25rem;
    border-bottom: 1px solid var(--line);
}

.ops__tab {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.72rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: var(--muted);
    background: transparent;
    border: none;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    padding: 0.6rem 1rem;
    cursor: pointer;
    transition:
        color 0.2s ease,
        border-color 0.2s ease;
}

.ops__tab:hover {
    color: var(--text);
}

.ops__tab--active {
    color: var(--text);
    border-bottom-color: var(--verdict);
}

.ops__tab-icon {
    width: 0.95rem;
    height: 0.95rem;
}

/* RANKED LIST */
.ops__list {
    list-style: none;
    margin: 0;
    padding: 0;
}

.ops__row {
    display: grid;
    grid-template-columns: auto minmax(0, 1.2fr) minmax(120px, 2fr) auto;
    gap: clamp(0.75rem, 1.5vw, 1.25rem);
    align-items: center;
    padding: 0.6rem 0;
    border-bottom: 1px dashed var(--line-soft);
    opacity: 0;
    animation: rowIn 0.45s both ease-out;
    animation-delay: calc(0.04s * var(--i) + 0.1s);
}

.ops__list--drawer .ops__row {
    grid-template-columns: minmax(0, 1fr) minmax(80px, 1.4fr) auto;
}

.ops__row:last-child {
    border-bottom: none;
}

.ops__row:hover .ops__domain {
    color: var(--text);
}

@keyframes rowIn {
    from {
        opacity: 0;
        transform: translateX(-8px);
    }
    to {
        opacity: 1;
        transform: translateX(0);
    }
}

.ops__rank {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.72rem;
    color: var(--dim);
    font-variant-numeric: tabular-nums;
}

.ops__domain {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.85rem;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    transition: color 0.2s ease;
}

.ops__bar {
    height: 3px;
    background: repeating-linear-gradient(90deg, var(--line-soft) 0 4px, transparent 4px 8px);
    position: relative;
    overflow: hidden;
}

.ops__bar-fill {
    height: 100%;
    width: var(--w);
    background: linear-gradient(
        90deg,
        var(--verdict),
        color-mix(in srgb, var(--verdict) 40%, var(--good))
    );
    transform: scaleX(0);
    transform-origin: left;
    animation: barGrow 0.8s both cubic-bezier(0.2, 0.7, 0.2, 1);
    animation-delay: calc(0.04s * var(--i) + 0.2s);
}

@keyframes barGrow {
    to {
        transform: scaleX(1);
    }
}

.ops__count {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.85rem;
    font-variant-numeric: tabular-nums;
    color: var(--text);
    text-align: right;
    min-width: 6ch;
}

/* DEVICES */
.ops__devs {
    list-style: none;
    margin: 0;
    padding: 0;
}

.ops__dev {
    display: grid;
    grid-template-columns: auto minmax(0, 1.6fr) auto auto minmax(0, 1fr) auto;
    gap: clamp(0.75rem, 1.5vw, 1.5rem);
    align-items: center;
    padding: 0.7rem 0.5rem;
    border-bottom: 1px dashed var(--line-soft);
    cursor: pointer;
    transition: background 0.18s ease;
}

.ops__dev:last-child {
    border-bottom: none;
}

.ops__dev:hover,
.ops__dev:focus-visible {
    background: color-mix(in srgb, var(--verdict) 7%, transparent);
    outline: none;
}

.ops__dev--active {
    background: color-mix(in srgb, var(--verdict) 12%, transparent);
}

.ops__dev-kind {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.58rem;
    letter-spacing: 0.12em;
    padding: 0.15rem 0.4rem;
    border: 1px solid var(--line);
    color: var(--muted);
}

.ops__dev-kind.is-mac {
    color: var(--good);
    border-color: color-mix(in srgb, var(--good) 45%, var(--line));
}

.ops__dev-id {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
}

.ops__dev-title {
    font-family: "Instrument Serif", serif;
    font-size: 1.05rem;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.ops__dev-sub {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.68rem;
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.ops__dev-stat,
.ops__dev-seen {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.85rem;
    font-variant-numeric: tabular-nums;
    text-align: right;
    color: var(--text);
}

.ops__dev-stat em,
.ops__dev-seen em {
    font-style: normal;
    font-size: 0.55rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: var(--muted);
}

.ops__dev-stat--good {
    color: var(--good);
}
.ops__dev-stat--bad {
    color: var(--accent);
}

.ops__dev-seen {
    font-size: 0.72rem;
    text-align: left;
}

.ops__dev-arrow {
    width: 1.1rem;
    height: 1.1rem;
    color: var(--dim);
    transition:
        transform 0.2s ease,
        color 0.2s ease;
}

.ops__dev:hover .ops__dev-arrow {
    transform: translateX(3px);
    color: var(--verdict);
}

/* STATES */
.ops__empty {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.75rem;
    letter-spacing: 0.15em;
    text-transform: uppercase;
    color: var(--muted);
    text-align: center;
    padding: 2rem 0;
}

.ops__skeleton {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    padding: 0.5rem 0;
}

.ops__skeleton span {
    height: 18px;
    background: linear-gradient(90deg, var(--line-soft), var(--line), var(--line-soft));
    background-size: 200% 100%;
    animation: shimmer 1.4s infinite;
}

@keyframes shimmer {
    0% {
        background-position: 200% 0;
    }
    100% {
        background-position: -200% 0;
    }
}

/* DRAWER BODY */
.ops-drawer {
    width: 100%;
    max-width: 32rem;
    font-family: "Instrument Serif", serif;
}

@media (max-width: 860px) {
    .ops__hero {
        grid-template-columns: 1fr;
    }
    .ops__hero-art {
        display: none;
    }
    /* Drop last-seen + allowed; keep kind | id | blocked | arrow = 4 cells. */
    .ops__dev {
        grid-template-columns: auto minmax(0, 1fr) auto auto;
    }
    .ops__dev-seen,
    .ops__dev-stat--good {
        display: none;
    }
}

@media (max-width: 600px) {
    .ops__row {
        grid-template-columns: auto minmax(0, 1fr) auto;
    }
    .ops__list--drawer .ops__row {
        grid-template-columns: minmax(0, 1fr) auto;
    }
    .ops__bar {
        display: none;
    }
    /* Drop the arrow affordance too; keep kind | id | blocked = 3 cells. */
    .ops__dev {
        grid-template-columns: auto minmax(0, 1fr) auto;
    }
    .ops__dev-arrow {
        display: none;
    }
}
</style>
