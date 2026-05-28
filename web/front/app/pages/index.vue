<script setup lang="ts">
import { api } from "~/api";
import { useComponentStatusWithLoading } from "~~/composables/use-component-status-with-loading";
import { getErrorMessage } from "~~/utils/get-error-message";

const toast = useToast();
const { isLoading, createLoadingRequest } = useComponentStatusWithLoading();

useHead({
    title: "Filter",
    link: [
        { rel: "preconnect", href: "https://fonts.googleapis.com" },
        { rel: "preconnect", href: "https://fonts.gstatic.com", crossorigin: "" },
        {
            rel: "stylesheet",
            href: "https://fonts.googleapis.com/css2?family=Instrument+Serif:ital@0;1&family=JetBrains+Mono:wght@300;400;500&display=swap",
        },
    ],
});

const PAUSE_OPTIONS: number[] = [5, 10, 15, 30];

const status = ref<boolean | null>(null);
const pausedUntil = ref<number>(0);
const nowUnix = ref<number>(Math.floor(Date.now() / 1000));
const selectedMinutes = ref<number>(PAUSE_OPTIONS[0]!);
const blockedTotal = ref<number | null>(null);
const blockedDisplay = ref(0);

let tickHandle: ReturnType<typeof setInterval> | null = null;

const isPaused = computed(() => pausedUntil.value > nowUnix.value);

const secondsLeft = computed(() => Math.max(0, pausedUntil.value - nowUnix.value));

const remainingLabel = computed(() => {
    const s = secondsLeft.value;
    const mm = String(Math.floor(s / 60)).padStart(2, "0");
    const ss = String(s % 60).padStart(2, "0");
    return `${mm}:${ss}`;
});

const pauseDurationSec = computed(() => {
    if (secondsLeft.value <= 0) return 0;
    const match = PAUSE_OPTIONS.find((m) => m * 60 >= secondsLeft.value);
    return (match ?? PAUSE_OPTIONS[PAUSE_OPTIONS.length - 1]!) * 60;
});

const pauseProgressPct = computed(() => {
    const total = pauseDurationSec.value;
    if (total <= 0) return 0;
    return Math.max(0, Math.min(100, (secondsLeft.value / total) * 100));
});

const stateKey = computed<"active" | "paused" | "disabled" | "unknown">(() => {
    if (status.value === null) return "unknown";
    if (!status.value) return "disabled";
    if (isPaused.value) return "paused";
    return "active";
});

const stateLabel = computed(() => {
    switch (stateKey.value) {
        case "active":
            return "Active";
        case "paused":
            return "Paused";
        case "disabled":
            return "Disabled";
        default:
            return "—";
    }
});

const orbActionLabel = computed(() => {
    switch (stateKey.value) {
        case "active":
            return "Disable filter";
        case "paused":
            return "Disable filter";
        case "disabled":
            return "Enable filter";
        default:
            return "Loading…";
    }
});

const startTicker = () => {
    if (tickHandle !== null) return;
    tickHandle = setInterval(() => {
        nowUnix.value = Math.floor(Date.now() / 1000);
        if (!isPaused.value && pausedUntil.value !== 0) {
            pausedUntil.value = 0;
            void fetchData();
        }
    }, 1000);
};

const stopTicker = () => {
    if (tickHandle !== null) {
        clearInterval(tickHandle);
        tickHandle = null;
    }
};

const applyResponse = (data: { status?: boolean; paused_until?: number }) => {
    status.value = data.status ?? false;
    pausedUntil.value = data.paused_until ?? 0;
    nowUnix.value = Math.floor(Date.now() / 1000);
    if (isPaused.value) startTicker();
    else stopTicker();
};

const showError = (error: unknown, title = "Error") => {
    toast.add({
        title,
        description: getErrorMessage(error),
        duration: 5000,
        color: "error",
    });
    console.error(title, error);
};

const fetchData = async () => {
    try {
        applyResponse(await api.getFilterStatus());
    } catch (error) {
        showError(error, "Failed to load filter status");
    }
};

const fetchDataWithLoading = createLoadingRequest(fetchData);

const animateNumber = (target: number, duration = 1400) => {
    const start = performance.now();
    const from = blockedDisplay.value;
    const tick = (t: number) => {
        const p = Math.min((t - start) / duration, 1);
        const eased = p === 1 ? 1 : 1 - Math.pow(2, -10 * p);
        blockedDisplay.value = Math.floor(from + (target - from) * eased);
        if (p < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
};

const fetchBlockedTotal = async () => {
    try {
        const res = await api.getBlockDomainsAmount();
        const value = res.amount ?? 0;
        blockedTotal.value = value;
        animateNumber(value);
    } catch (error) {
        console.error("Failed to load blocked total", error);
    }
};

onMounted(() => {
    void fetchDataWithLoading();
    void fetchBlockedTotal();
});
onBeforeUnmount(stopTicker);

const changeStatus = async () => {
    try {
        applyResponse(await api.changeFilterStatus());
    } catch (error) {
        showError(error, "Failed to change filter status");
    }
};

const pauseFilter = async () => {
    try {
        applyResponse(await api.pauseFilter(selectedMinutes.value));
    } catch (error) {
        showError(error, "Failed to pause filter");
    }
};

const resumeFilter = async () => {
    try {
        applyResponse(await api.resumeFilter());
    } catch (error) {
        showError(error, "Failed to resume filter");
    }
};

const changeStatusWithLoading = createLoadingRequest(changeStatus);
const pauseFilterWithLoading = createLoadingRequest(pauseFilter);
const resumeFilterWithLoading = createLoadingRequest(resumeFilter);

const pauseSelectItems = PAUSE_OPTIONS.map((m) => ({
    label: `${m} min`,
    value: m,
}));

const formatNumber = (n: number) => n.toLocaleString("en-US");

const now = new Date();
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
</script>

<template>
    <div class="ops" :data-state="stateKey">
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
                        <span>Filter control</span>
                        <span class="ops__eyebrow-dot"></span>
                        <span>global state</span>
                    </p>

                    <h1 class="ops__metric">
                        <Transition name="state-swap" appear>
                            <span :key="stateKey" class="ops__metric-value">{{ stateLabel }}</span>
                        </Transition>
                    </h1>

                    <div class="ops__rule"></div>

                    <dl class="ops__hero-meta">
                        <div>
                            <dt>Blocked total</dt>
                            <dd>
                                <span v-if="blockedTotal !== null">{{
                                    formatNumber(blockedDisplay)
                                }}</span>
                                <span v-else class="ops__meta-skel"></span>
                            </dd>
                        </div>
                        <div>
                            <dt>Resolver</dt>
                            <dd>UDP/TCP&nbsp;:53</dd>
                        </div>
                        <div>
                            <dt>Mode</dt>
                            <dd>NXDOMAIN sink</dd>
                        </div>
                    </dl>

                    <div v-if="status" class="ops__pause">
                        <span class="ops__pause-label">
                            <span
                                class="ops__pause-dot"
                                :class="{ 'ops__pause-dot--hidden': !isPaused }"
                            ></span>
                            {{ isPaused ? "Resuming in" : "Pause filter for" }}
                        </span>
                        <div class="ops__pause-row">
                            <div class="ops__pause-slot">
                                <div v-show="!isPaused" class="ops__pause-slot-item">
                                    <USelect
                                        v-model="selectedMinutes"
                                        :items="pauseSelectItems"
                                        value-key="value"
                                        size="md"
                                        class="ops__pause-select"
                                    />
                                </div>
                                <div v-show="isPaused" class="ops__pause-slot-item">
                                    <div class="ops__pause-clockwrap">
                                        <span class="ops__pause-clock">{{ remainingLabel }}</span>
                                        <div
                                            class="ops__pause-progress"
                                            role="progressbar"
                                            :aria-valuenow="pauseProgressPct"
                                            aria-valuemin="0"
                                            aria-valuemax="100"
                                        >
                                            <div
                                                class="ops__pause-progress-fill"
                                                :style="{ width: pauseProgressPct + '%' }"
                                            ></div>
                                        </div>
                                    </div>
                                </div>
                            </div>
                            <button
                                type="button"
                                class="ops__cmd"
                                :disabled="isLoading"
                                @click="
                                    isPaused ? resumeFilterWithLoading() : pauseFilterWithLoading()
                                "
                            >
                                <span class="ops__cmd-corner ops__cmd-corner--tl"></span>
                                <span class="ops__cmd-corner ops__cmd-corner--tr"></span>
                                <span class="ops__cmd-corner ops__cmd-corner--bl"></span>
                                <span class="ops__cmd-corner ops__cmd-corner--br"></span>
                                <UIcon
                                    :name="
                                        isLoading
                                            ? 'i-lucide-loader'
                                            : isPaused
                                              ? 'i-lucide-play'
                                              : 'i-lucide-pause'
                                    "
                                    class="ops__cmd-icon"
                                    :class="{ 'ops__cmd-icon--spin': isLoading }"
                                />
                                <span class="ops__cmd-label">{{
                                    isPaused ? "Resume now" : "Pause"
                                }}</span>
                                <UIcon name="i-lucide-arrow-right" class="ops__cmd-arrow" />
                            </button>
                        </div>
                    </div>
                </div>

                <div class="ops__hero-art" :class="{ 'ops__hero-art--ready': status !== null }">
                    <button
                        type="button"
                        class="ops__orb"
                        :class="`ops__orb--${stateKey}`"
                        :disabled="status === null || isLoading"
                        :aria-label="orbActionLabel"
                        :title="orbActionLabel"
                        @click="changeStatusWithLoading"
                    >
                        <div class="ops__orb-pulse ops__orb-pulse--a"></div>
                        <div class="ops__orb-pulse ops__orb-pulse--b"></div>
                        <div class="ops__orb-pulse ops__orb-pulse--c"></div>
                        <div class="ops__orb-core">
                            <UIcon
                                v-if="isLoading"
                                name="i-lucide-loader"
                                class="ops__orb-icon ops__orb-icon--spin"
                            />
                            <UIcon
                                v-else-if="stateKey === 'active'"
                                name="i-lucide-shield-check"
                                class="ops__orb-icon"
                            />
                            <UIcon
                                v-else-if="stateKey === 'paused'"
                                name="i-lucide-shield-alert"
                                class="ops__orb-icon"
                            />
                            <UIcon
                                v-else-if="stateKey === 'disabled'"
                                name="i-lucide-shield-off"
                                class="ops__orb-icon"
                            />
                            <UIcon
                                v-else
                                name="i-lucide-loader"
                                class="ops__orb-icon ops__orb-icon--spin"
                            />
                        </div>
                        <span class="ops__orb-action">{{ orbActionLabel }}</span>
                    </button>
                    <span class="ops__hero-art-coord ops__hero-art-coord--tl">RES.0 / SINK</span>
                    <span class="ops__hero-art-coord ops__hero-art-coord--br">{{
                        stateLabel.toUpperCase()
                    }}</span>
                </div>
            </section>

            <footer class="ops__foot">
                <span>SINKHOLE · NXDOMAIN · UDP/TCP:53</span>
                <span class="ops__foot-mid">DoH UPSTREAM · CLOUDFLARE</span>
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
    --warn: var(--ui-color-warning-500);

    --state: var(--good);

    min-height: calc(100vh - var(--ui-header-height));
    background:
        radial-gradient(
            1200px 600px at 75% -10%,
            color-mix(in srgb, var(--state) 9%, transparent),
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
    transition: background 0.6s ease;
}

.ops[data-state="active"] {
    --state: var(--good);
}
.ops[data-state="paused"] {
    --state: var(--warn);
}
.ops[data-state="disabled"] {
    --state: var(--accent);
}
.ops[data-state="unknown"] {
    --state: var(--dim);
}

.ops__inner {
    display: grid;
    grid-template-rows: auto 1fr auto;
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
    justify-content: space-between;
    align-items: center;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.7rem;
    letter-spacing: 0.16em;
    text-transform: uppercase;
    color: var(--muted);
    padding-bottom: 0.875rem;
    border-bottom: 1px solid var(--line);
}

.ops__top-left,
.ops__top-right {
    display: flex;
    gap: 0.625rem;
    align-items: center;
}

.ops__top-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--state);
    box-shadow: 0 0 0 4px color-mix(in srgb, var(--state) 18%, transparent);
    animation: pulseDot 2.4s ease-in-out infinite;
}

@keyframes pulseDot {
    0%,
    100% {
        opacity: 1;
        transform: scale(1);
    }
    50% {
        opacity: 0.55;
        transform: scale(0.85);
    }
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
    min-height: 0.9em;
    position: relative;
}

.state-swap-enter-active,
.state-swap-leave-active {
    transition: opacity 0.28s linear;
    animation: none !important;
    will-change: opacity;
}

.state-swap-leave-active {
    position: absolute;
    left: 0;
    top: 0;
    right: 0;
}

.state-swap-enter-from,
.state-swap-leave-to {
    opacity: 0;
}

.ops__metric-value {
    display: inline-block;
    font-variant-numeric: tabular-nums;
    color: var(--text);
}

.ops__rule {
    width: clamp(140px, 18vw, 240px);
    height: 1px;
    background: linear-gradient(90deg, var(--state), var(--line) 70%, transparent);
    margin: 1.5rem 0 1rem;
    animation: ruleGrow 0.9s 0.5s both cubic-bezier(0.2, 0.7, 0.2, 1);
    transform-origin: left;
    transition: background 0.5s ease;
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

.ops__meta-skel {
    display: inline-block;
    width: 5ch;
    height: 0.85em;
    background: linear-gradient(90deg, var(--line-soft), var(--line), var(--line-soft));
    background-size: 200% 100%;
    animation: shimmer 1.4s infinite;
}

/* HERO ART — pulsing orb */
.ops__hero-art {
    position: relative;
    width: clamp(180px, 18vw, 280px);
    aspect-ratio: 1;
    color: var(--state);
    opacity: 0;
    transition:
        opacity 0.8s 0.6s ease,
        color 0.5s ease;
    display: grid;
    place-items: center;
}

.ops__hero-art--ready {
    opacity: 1;
}

.ops__orb {
    position: relative;
    width: 100%;
    height: 100%;
    display: grid;
    place-items: center;
    background: transparent;
    border: none;
    padding: 0;
    margin: 0;
    color: inherit;
    cursor: pointer;
    border-radius: 50%;
    outline: none;
    transition: transform 0.25s ease;
}

.ops__orb:hover:not(:disabled) {
    transform: scale(1.04);
}

.ops__orb:active:not(:disabled) {
    transform: scale(0.97);
}

.ops__orb:focus-visible {
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--state) 60%, transparent);
}

.ops__orb:disabled {
    cursor: progress;
}

.ops__orb:hover:not(:disabled) .ops__orb-core {
    border-color: var(--state);
    box-shadow:
        inset 0 0 32px color-mix(in srgb, var(--state) 28%, transparent),
        0 0 56px color-mix(in srgb, var(--state) 22%, transparent);
}

.ops__orb:hover:not(:disabled) .ops__orb-action {
    opacity: 1;
    transform: translate(-50%, 0);
}

.ops__orb-action {
    position: absolute;
    left: 50%;
    bottom: -1.75rem;
    transform: translate(-50%, -4px);
    font-family: "JetBrains Mono", monospace;
    font-size: 0.62rem;
    letter-spacing: 0.2em;
    text-transform: uppercase;
    color: var(--state);
    white-space: nowrap;
    opacity: 0;
    transition:
        opacity 0.25s ease,
        transform 0.25s ease;
    pointer-events: none;
}

.ops__orb-pulse {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    border: 1px solid color-mix(in srgb, var(--state) 60%, transparent);
    opacity: 0;
    animation: orbPulse 3.6s ease-out infinite;
}

.ops__orb-pulse--a {
    animation-delay: 0s;
}
.ops__orb-pulse--b {
    animation-delay: 1.2s;
}
.ops__orb-pulse--c {
    animation-delay: 2.4s;
}

.ops[data-state="paused"] .ops__orb-pulse,
.ops[data-state="disabled"] .ops__orb-pulse {
    animation-duration: 5s;
}

@keyframes orbPulse {
    0% {
        transform: scale(0.45);
        opacity: 0.9;
    }
    80% {
        opacity: 0;
    }
    100% {
        transform: scale(1);
        opacity: 0;
    }
}

.ops__orb-core {
    width: 44%;
    height: 44%;
    border-radius: 50%;
    background: radial-gradient(
        circle at 30% 30%,
        color-mix(in srgb, var(--state) 35%, var(--bg-elev)),
        var(--bg-elev) 70%
    );
    border: 1px solid color-mix(in srgb, var(--state) 50%, var(--line));
    display: grid;
    place-items: center;
    box-shadow:
        inset 0 0 24px color-mix(in srgb, var(--state) 20%, transparent),
        0 0 40px color-mix(in srgb, var(--state) 14%, transparent);
}

.ops__orb-icon {
    width: 38%;
    height: 38%;
    color: var(--state);
}

.ops__orb-icon--spin {
    animation: spin 1.6s linear infinite;
}

@keyframes spin {
    to {
        transform: rotate(360deg);
    }
}

.ops__hero-art-coord {
    position: absolute;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.6rem;
    letter-spacing: 0.15em;
    color: var(--muted);
    white-space: nowrap;
    text-transform: uppercase;
}

.ops__hero-art-coord--tl {
    top: -0.5rem;
    left: -1rem;
}

.ops__hero-art-coord--br {
    bottom: -0.5rem;
    right: -1rem;
    color: var(--state);
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

/* INLINE PAUSE CONTROLS */
.ops__pause {
    --pause-accent: var(--warn);
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.75rem;
    margin-top: 1.75rem;
    animation: fadeUp 0.5s 0.65s both ease-out;
    transition: --pause-accent 0.4s ease;
}

.ops__pause::before {
    content: "";
    display: block;
    width: calc(11rem + 0.75rem + 13rem);
    max-width: 100%;
    border-top: 1px dashed var(--line-soft);
    margin-bottom: 1.25rem;
}

.ops[data-state="paused"] .ops__pause {
    --pause-accent: var(--good);
}

.ops__pause-label {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
    font-family: "JetBrains Mono", monospace;
    font-size: 0.72rem;
    letter-spacing: 0.22em;
    text-transform: uppercase;
    color: var(--muted);
}

.ops__pause-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--pause-accent);
    box-shadow: 0 0 0 4px color-mix(in srgb, var(--pause-accent) 18%, transparent);
    animation: pulseDot 2s ease-in-out infinite;
    transition:
        opacity 0.25s ease,
        background 0.4s ease,
        box-shadow 0.4s ease;
}

.ops__pause-dot--hidden {
    opacity: 0;
    animation: none;
}

.ops__pause-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
}

.ops__pause-slot {
    position: relative;
    width: 11rem;
    min-height: 2.4rem;
}

.ops__pause-slot-item {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    width: 100%;
}

.ops__pause-slot-item > * {
    width: 100%;
}

.ops__pause-clock {
    font-family: "JetBrains Mono", monospace;
    font-size: 1.75rem;
    line-height: 1;
    color: var(--pause-accent);
    font-variant-numeric: tabular-nums;
    letter-spacing: 0.04em;
    min-width: 4.5ch;
    transition: color 0.4s ease;
}

.ops__pause-select {
    width: 100%;
}

.ops__pause-clockwrap {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
}

.ops__pause-progress {
    height: 3px;
    background: repeating-linear-gradient(90deg, var(--line-soft) 0 4px, transparent 4px 8px);
    overflow: hidden;
}

.ops__pause-progress-fill {
    height: 100%;
    background: linear-gradient(
        90deg,
        var(--pause-accent),
        color-mix(in srgb, var(--pause-accent) 60%, var(--text))
    );
    transition:
        width 1s linear,
        background 0.4s ease;
}

/* COMMAND BUTTON */
.ops__cmd {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: flex-start;
    gap: 0.625rem;
    padding: 0.4rem 1.25rem 0.4rem 1.1rem;
    width: 13rem;
    background: var(--bg-elev);
    border: 1px solid var(--line);
    color: var(--text);
    font-family: "JetBrains Mono", monospace;
    font-size: 0.78rem;
    letter-spacing: 0.18em;
    text-transform: uppercase;
    white-space: nowrap;
    cursor: pointer;
    overflow: hidden;
    isolation: isolate;
    transition:
        color 0.25s ease,
        border-color 0.25s ease,
        transform 0.15s ease;
}

.ops__cmd::before {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(
        90deg,
        color-mix(in srgb, var(--pause-accent) 22%, transparent),
        color-mix(in srgb, var(--pause-accent) 6%, transparent)
    );
    transform: translateX(-101%);
    transition: transform 0.4s cubic-bezier(0.2, 0.7, 0.2, 1);
    z-index: -1;
}

.ops__cmd:hover:not(:disabled) {
    border-color: var(--pause-accent);
    color: var(--text);
}

.ops__cmd:hover:not(:disabled)::before {
    transform: translateX(0);
}

.ops__cmd:active:not(:disabled) {
    transform: translateY(1px);
}

.ops__cmd:focus-visible {
    outline: none;
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--pause-accent) 50%, transparent);
}

.ops__cmd:disabled {
    cursor: progress;
    opacity: 0.7;
}

.ops__cmd-corner {
    position: absolute;
    width: 8px;
    height: 8px;
    border: 1px solid var(--pause-accent);
    transition: border-color 0.4s ease;
}
.ops__cmd-corner--tl {
    top: -1px;
    left: -1px;
    border-right: none;
    border-bottom: none;
}
.ops__cmd-corner--tr {
    top: -1px;
    right: -1px;
    border-left: none;
    border-bottom: none;
}
.ops__cmd-corner--bl {
    bottom: -1px;
    left: -1px;
    border-right: none;
    border-top: none;
}
.ops__cmd-corner--br {
    bottom: -1px;
    right: -1px;
    border-left: none;
    border-top: none;
}

.ops__cmd-icon {
    flex-shrink: 0;
    width: 1.05rem;
    height: 1.05rem;
    color: var(--pause-accent);
    transition:
        transform 0.25s ease,
        color 0.4s ease;
}

.ops__cmd-icon--spin {
    animation: spin 1.4s linear infinite;
}

.ops__cmd-label {
    flex: 1;
    line-height: 1;
    text-align: left;
}

.ops__cmd-arrow {
    flex-shrink: 0;
    width: 0.95rem;
    height: 0.95rem;
    color: var(--dim);
    margin-left: 0.25rem;
    transition:
        transform 0.25s ease,
        color 0.25s ease;
}

.ops__cmd:hover:not(:disabled) .ops__cmd-arrow {
    transform: translateX(4px);
    color: var(--pause-accent);
}

.ops__cmd:hover:not(:disabled) .ops__cmd-icon {
    transform: scale(1.1);
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
        order: -1;
        width: clamp(140px, 40vw, 200px);
        margin: 0 auto;
    }
    .ops__hero-art-coord {
        display: none;
    }
}
</style>
