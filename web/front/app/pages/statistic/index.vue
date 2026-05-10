<script setup lang="ts">
import { api } from "~/api";
import type { DbDomainCount } from "~/api/generated/data-contracts";

useHead({
    title: "Statistic",
    link: [
        { rel: "preconnect", href: "https://fonts.googleapis.com" },
        { rel: "preconnect", href: "https://fonts.gstatic.com", crossorigin: "" },
        {
            rel: "stylesheet",
            href: "https://fonts.googleapis.com/css2?family=Instrument+Serif:ital@0;1&family=JetBrains+Mono:wght@300;400;500&display=swap",
        },
    ],
});

const totalAmount = ref(0);
const displayAmount = ref(0);
const groups = ref<DbDomainCount[]>([]);
const sourcesCount = ref<number | null>(null);
const ready = ref(false);
const now = new Date();

const formatNumber = (n: number) => n.toLocaleString("en-US");

const animate = (target: number, duration = 1800) => {
    const start = performance.now();
    const tick = (t: number) => {
        const p = Math.min((t - start) / duration, 1);
        const eased = p === 1 ? 1 : 1 - Math.pow(2, -10 * p);
        displayAmount.value = Math.floor(target * eased);
        if (p < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
};

const topGroups = computed(() =>
    [...groups.value].sort((a, b) => (b.count ?? 0) - (a.count ?? 0)).slice(0, 8),
);

const maxCount = computed(() => Math.max(...topGroups.value.map((g) => g.count ?? 0), 1));

const totalTop = computed(() => topGroups.value.reduce((acc, g) => acc + (g.count ?? 0), 0));

const formattedDate = (() => {
    const y = now.getFullYear();
    const m = String(now.getMonth() + 1).padStart(2, "0");
    const d = String(now.getDate()).padStart(2, "0");
    return `${y}.${m}.${d}`;
})();

const formattedTime = (() => {
    const h = String(now.getHours()).padStart(2, "0");
    const m = String(now.getMinutes()).padStart(2, "0");
    return `${h}:${m}`;
})();

onMounted(async () => {
    const sourcesController = new AbortController();
    try {
        const [amountRes, groupsRes, sourcesRes] = await Promise.all([
            api.getBlockDomainsAmount(),
            api.getBlockDomainsGroups(),
            api.getAllSyncRecords(sourcesController.signal),
        ]);
        totalAmount.value = amountRes.amount ?? 0;
        groups.value = groupsRes.groups ?? [];
        sourcesCount.value = sourcesRes.total ?? sourcesRes.list?.length ?? 0;
        ready.value = true;
        animate(totalAmount.value);
    } catch (e) {
        console.error("Statistic load failed", e);
    }
});
</script>

<template>
    <div class="ops">
        <div class="ops__grid" aria-hidden="true"></div>

        <UContainer class="ops__inner">
            <header class="ops__top">
                <div class="ops__top-right">
                    <span>{{ formattedDate }} &nbsp;{{ formattedTime }}</span>
                </div>
            </header>

            <section class="ops__hero">
                <div class="ops__hero-text">
                    <p class="ops__eyebrow">
                        <span>Cumulative ledger</span>
                        <span class="ops__eyebrow-dot"></span>
                        <span>since uptime</span>
                    </p>

                    <h1 class="ops__metric">
                        <span class="ops__metric-value">{{ formatNumber(displayAmount) }}</span>
                    </h1>

                    <p class="ops__metric-label">
                        <em>domains</em> intercepted &amp; resolved as
                        <span class="ops__chip">NXDOMAIN</span> at the sinkhole
                    </p>

                    <div class="ops__rule"></div>

                    <dl class="ops__hero-meta">
                        <div>
                            <dt>Top&nbsp;08 sum</dt>
                            <dd>{{ formatNumber(totalTop) }}</dd>
                        </div>
                        <div>
                            <dt>Sources</dt>
                            <dd>
                                <span v-if="sourcesCount !== null">{{
                                    String(sourcesCount).padStart(2, "0")
                                }}</span>
                                <span v-else>—</span>
                            </dd>
                        </div>
                    </dl>
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
                        <text x="120" y="22" text-anchor="middle" class="ops__hero-art-tick">
                            N
                        </text>
                        <text x="120" y="226" text-anchor="middle" class="ops__hero-art-tick">
                            S
                        </text>
                        <text x="14" y="124" text-anchor="middle" class="ops__hero-art-tick">
                            W
                        </text>
                        <text x="226" y="124" text-anchor="middle" class="ops__hero-art-tick">
                            E
                        </text>
                    </svg>
                    <span class="ops__hero-art-coord ops__hero-art-coord--tl"
                        >52.34°N / 13.41°E</span
                    >
                    <span class="ops__hero-art-coord ops__hero-art-coord--br"
                        >RES.0 / 192.168.0.1</span
                    >
                </div>
            </section>

            <section class="ops__panel">
                <div class="ops__panel-corner ops__panel-corner--tl"></div>
                <div class="ops__panel-corner ops__panel-corner--tr"></div>
                <div class="ops__panel-corner ops__panel-corner--bl"></div>
                <div class="ops__panel-corner ops__panel-corner--br"></div>

                <header class="ops__panel-head">
                    <div>
                        <h2><em>Top targets</em></h2>
                        <p>most-resolved blocks · ranked top 08</p>
                    </div>
                    <span class="ops__panel-axis"
                        >0 ─────────────────── {{ formatNumber(maxCount) }}</span
                    >
                </header>

                <ol v-if="ready && topGroups.length" class="ops__list">
                    <li
                        v-for="(g, i) in topGroups"
                        :key="g.domain ?? i"
                        class="ops__row"
                        :style="{ '--i': i, '--w': `${((g.count ?? 0) / maxCount) * 100}%` }"
                    >
                        <span class="ops__rank">{{ String(i + 1).padStart(2, "0") }}</span>
                        <span class="ops__domain" :title="g.domain ?? ''">{{ g.domain }}</span>
                        <div class="ops__bar">
                            <div class="ops__bar-fill"></div>
                        </div>
                        <span class="ops__count">{{ formatNumber(g.count ?? 0) }}</span>
                    </li>
                </ol>
                <p v-else-if="ready" class="ops__empty">no signal · ledger empty</p>
                <div v-else class="ops__skeleton">
                    <span v-for="n in 6" :key="n"></span>
                </div>
            </section>

            <footer class="ops__foot">
                <span class="ops__foot-mid">SINKHOLE · NXDOMAIN · UDP/TCP:53</span>
                <span>SRC: STEVENBLACK · EASYLIST · USER</span>
            </footer>
        </UContainer>
    </div>
</template>

<style scoped>
.ops {
    --bg: var(--ui-bg);
    --bg-elev: var(--ui-bg-elevated);
    --bg-accent: var(--ui-bg-accented);
    --line: var(--ui-border);
    --line-soft: var(--ui-border-muted);
    --text: var(--ui-text-highlighted);
    --body: var(--ui-text);
    --muted: var(--ui-text-muted);
    --dim: var(--ui-text-dimmed);
    --accent: var(--ui-color-error-500);
    --accent-soft: var(--ui-color-error-400);
    --good: var(--ui-color-primary-500);

    min-height: calc(100vh - var(--ui-header-height));
    background:
        radial-gradient(
            1200px 600px at 75% -10%,
            color-mix(in srgb, var(--accent) 8%, transparent),
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
}

.ops__inner {
    display: grid;
    grid-template-rows: auto 1fr auto auto;
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

/* TOP BAR */
.ops__top {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.7rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: var(--muted);
    padding-bottom: 0.875rem;
    border-bottom: 1px solid var(--line);
}

.ops__top-right {
    display: flex;
    gap: 0.875rem;
    align-items: center;
}

/* HERO */
.ops__hero {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: clamp(1.5rem, 4vw, 3rem);
    padding: clamp(0.5rem, 2vw, 1.5rem) 0;
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
    animation: fadeUp 0.6s 0.1s both ease-out;
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
    animation: fadeUp 0.9s 0.2s both ease-out;
}

.ops__metric-label {
    font-size: clamp(1.05rem, 1.5vw, 1.3rem);
    color: var(--muted);
    line-height: 1.45;
    max-width: 36ch;
    margin: 1.25rem 0 0;
    animation: fadeUp 0.6s 0.4s both ease-out;
}

.ops__metric-label em {
    font-style: italic;
    color: var(--accent);
    letter-spacing: -0.01em;
}

.ops__chip {
    display: inline-block;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.7rem;
    letter-spacing: 0.1em;
    color: var(--text);
    background: var(--bg-elev);
    border: 1px solid var(--line);
    padding: 0.1rem 0.45rem;
    transform: translateY(-2px);
}

.ops__rule {
    width: clamp(140px, 18vw, 240px);
    height: 1px;
    background: linear-gradient(90deg, var(--accent), var(--line) 70%, transparent);
    margin: 1.5rem 0 1rem;
    animation: ruleGrow 0.9s 0.5s both cubic-bezier(0.2, 0.7, 0.2, 1);
    transform-origin: left;
}

@keyframes ruleGrow {
    from {
        transform: scaleX(0);
    }
    to {
        transform: scaleX(1);
    }
}

.ops__hero-meta {
    display: flex;
    flex-wrap: wrap;
    gap: clamp(1rem, 3vw, 2.25rem);
    margin: 0;
    animation: fadeUp 0.6s 0.55s both ease-out;
}

.ops__hero-meta div {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
}

.ops__hero-meta dt {
    font-family: "JetBrains Mono", monospace;
    font-size: 0.62rem;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    color: var(--muted);
}

.ops__hero-meta dd {
    margin: 0;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.95rem;
    color: var(--text);
    font-variant-numeric: tabular-nums;
}

.ops__hero-art {
    position: relative;
    width: clamp(180px, 18vw, 280px);
    aspect-ratio: 1;
    color: var(--dim);
    opacity: 0;
    transition: opacity 0.8s 0.6s ease;
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
    color: var(--accent);
}

@keyframes spin {
    to {
        transform: rotate(360deg);
    }
}

.ops__hero-art-tick {
    font-family: "JetBrains Mono", monospace;
    font-size: 7px;
    letter-spacing: 0.1em;
    fill: var(--muted);
}

.ops__hero-art-coord {
    position: absolute;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.6rem;
    letter-spacing: 0.15em;
    color: var(--muted);
    white-space: nowrap;
}

.ops__hero-art-coord--tl {
    top: -0.5rem;
    left: -1rem;
}

.ops__hero-art-coord--br {
    bottom: -0.5rem;
    right: -1rem;
}

@keyframes fadeUp {
    from {
        opacity: 0;
        transform: translateY(14px);
    }
    to {
        opacity: 1;
        transform: translateY(0);
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
    box-shadow: 0 1px 0 0 color-mix(in srgb, var(--text) 4%, transparent);
}

.ops__panel-corner {
    position: absolute;
    width: 10px;
    height: 10px;
    border: 1px solid var(--accent);
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
    color: var(--text);
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

.ops__list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
}

.ops__row {
    display: grid;
    grid-template-columns: auto minmax(0, 1.2fr) minmax(120px, 2fr) auto;
    gap: clamp(0.75rem, 1.5vw, 1.25rem);
    align-items: center;
    padding: 0.6rem 0;
    border-bottom: 1px dashed var(--line-soft);
    opacity: 0;
    animation: rowIn 0.5s both ease-out;
    animation-delay: calc(0.06s * var(--i) + 0.35s);
}

.ops__row:last-child {
    border-bottom: none;
}

.ops__row:hover .ops__bar-fill {
    background: linear-gradient(90deg, var(--good), var(--accent));
}

.ops__row:hover .ops__domain {
    color: var(--good);
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
    color: var(--text);
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
    background: linear-gradient(90deg, var(--accent), var(--good));
    transform: scaleX(0);
    transform-origin: left;
    animation: barGrow 0.9s both cubic-bezier(0.2, 0.7, 0.2, 1);
    animation-delay: calc(0.06s * var(--i) + 0.5s);
    transition: background 0.25s ease;
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

/* FOOTER */
.ops__foot {
    display: flex;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 0.75rem 1.5rem;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.62rem;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    color: var(--muted);
    padding-top: 0.875rem;
    border-top: 1px solid var(--line);
}

.ops__foot-mid {
    color: var(--dim);
}

@media (max-width: 860px) {
    .ops__hero {
        grid-template-columns: 1fr;
    }
    .ops__hero-art {
        display: none;
    }
}

@media (max-width: 600px) {
    .ops__row {
        grid-template-columns: auto minmax(0, 1fr) auto;
    }
    .ops__bar {
        display: none;
    }
    .ops__panel-axis {
        display: none;
    }
}
</style>
