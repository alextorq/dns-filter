<script setup lang="ts">
import * as z from "zod";
import type { FormSubmitEvent } from "@nuxt/ui";
import { getErrorMessage } from "~~/utils/get-error-message";
import { useAuth } from "~~/composables/use-auth";

definePageMeta({ layout: "auth" });

useHead({
    title: "Auth — DNS Filter",
});

const schema = z.object({
    login: z.string().min(1, "Required"),
    password: z.string().min(1, "Required"),
});

type Schema = z.output<typeof schema>;

const state = reactive({
    login: "",
    password: "",
});

const toast = useToast();
const { login } = useAuth();
const isSubmitting = ref(false);

async function onSubmit(payload: FormSubmitEvent<Schema>) {
    isSubmitting.value = true;
    try {
        await login(payload.data.login, payload.data.password);
        await navigateTo("/");
    } catch (error) {
        toast.add({
            title: "Authentication failed",
            description: getErrorMessage(error),
            duration: 4000,
            color: "error",
        });
    } finally {
        isSubmitting.value = false;
    }
}

const blockedSamples = [
    { d: "doubleclick.net", t: "tracker" },
    { d: "googletagmanager.com", t: "analytics" },
    { d: "scorecardresearch.com", t: "tracker" },
    { d: "amazon-adsystem.com", t: "ads" },
    { d: "googlesyndication.com", t: "ads" },
    { d: "criteo.com", t: "tracker" },
    { d: "adsrvr.org", t: "ads" },
    { d: "facebook.com/tr", t: "tracker" },
    { d: "taboola.com", t: "ads" },
    { d: "outbrain.com", t: "tracker" },
];
</script>

<template>
    <div
        class="auth-page bg-default text-default relative grid min-h-dvh grid-rows-[auto_1fr_auto] overflow-hidden"
    >
        <div class="bg-grid pointer-events-none absolute inset-0" aria-hidden="true"></div>
        <div class="bg-glow pointer-events-none absolute" aria-hidden="true"></div>

        <header
            class="border-default relative z-10 flex items-center justify-between border-b px-6 py-4 sm:px-8"
        >
            <AppLogo />
            <UColorModeButton />
        </header>

        <div
            class="relative z-10 mx-auto grid w-full max-w-6xl items-center gap-10 px-6 py-8 sm:px-8 lg:grid-cols-2 lg:gap-14"
        >
            <aside class="visual flex flex-col gap-4 max-lg:hidden">
                <div
                    class="border-default bg-elevated/40 relative overflow-hidden rounded-lg border px-5 py-5 backdrop-blur"
                >
                    <div
                        class="via-primary/60 absolute top-0 right-0 left-0 h-px bg-gradient-to-r from-transparent to-transparent"
                    ></div>

                    <div
                        class="mb-4 flex items-center justify-between font-mono text-[11px] tracking-wider"
                    >
                        <span class="text-primary font-medium">// LIVE FILTER</span>
                        <span class="text-muted">:53/udp · doh-upstream</span>
                    </div>

                    <svg
                        class="topology mb-3 w-full"
                        viewBox="0 0 360 130"
                        fill="none"
                        aria-hidden="true"
                    >
                        <circle cx="40" cy="65" r="6" class="svg-stroke-muted" stroke-width="1.4" />
                        <text
                            x="40"
                            y="92"
                            text-anchor="middle"
                            font-family="ui-monospace, monospace"
                            font-size="9"
                            class="svg-text-dimmed"
                        >
                            client
                        </text>

                        <path
                            d="M 50 65 L 165 65"
                            class="svg-stroke-faint"
                            stroke-width="1"
                            stroke-dasharray="2 3"
                        />

                        <circle
                            cx="184"
                            cy="65"
                            r="16"
                            class="svg-stroke-primary svg-fill-primary-soft"
                            stroke-width="1.4"
                        />
                        <circle cx="184" cy="65" r="3" class="svg-fill-primary pulse-node" />
                        <text
                            x="184"
                            y="100"
                            text-anchor="middle"
                            font-family="ui-monospace, monospace"
                            font-size="9"
                            class="svg-text-primary"
                        >
                            filter
                        </text>

                        <path
                            d="M 200 58 L 285 28"
                            class="svg-stroke-success"
                            stroke-width="1"
                            stroke-dasharray="2 3"
                        />
                        <path
                            d="M 200 72 L 285 102"
                            class="svg-stroke-error"
                            stroke-width="1"
                            stroke-dasharray="2 3"
                        />

                        <circle
                            cx="295"
                            cy="25"
                            r="5"
                            class="svg-stroke-success svg-fill-success-soft"
                            stroke-width="1.4"
                        />
                        <text
                            x="306"
                            y="29"
                            font-family="ui-monospace, monospace"
                            font-size="9"
                            class="svg-text-success"
                        >
                            allow
                        </text>

                        <circle
                            cx="295"
                            cy="105"
                            r="5"
                            class="svg-stroke-error svg-fill-error-soft"
                            stroke-width="1.4"
                        />
                        <text
                            x="306"
                            y="109"
                            font-family="ui-monospace, monospace"
                            font-size="9"
                            class="svg-text-error"
                        >
                            block
                        </text>

                        <circle r="2.6" class="svg-fill-primary">
                            <animateMotion
                                dur="2.4s"
                                repeatCount="indefinite"
                                path="M 50 65 L 165 65 L 184 65 L 285 28"
                            />
                            <animate
                                attributeName="opacity"
                                values="0;1;1;0"
                                keyTimes="0;0.1;0.9;1"
                                dur="2.4s"
                                repeatCount="indefinite"
                            />
                        </circle>
                        <circle r="2.6" class="svg-fill-error">
                            <animateMotion
                                dur="2s"
                                begin="0.9s"
                                repeatCount="indefinite"
                                path="M 50 65 L 165 65 L 184 65 L 285 102"
                            />
                            <animate
                                attributeName="opacity"
                                values="0;1;1;0"
                                keyTimes="0;0.1;0.9;1"
                                dur="2s"
                                begin="0.9s"
                                repeatCount="indefinite"
                            />
                        </circle>
                    </svg>

                    <div
                        class="border-default relative h-[168px] overflow-hidden border-t border-dashed pt-3"
                    >
                        <ul class="m-0 flex list-none flex-col gap-1.5 p-0">
                            <li
                                v-for="(item, i) in blockedSamples"
                                :key="item.d"
                                class="stream-item text-muted grid grid-cols-[auto_auto_1fr_auto] items-center gap-3 font-mono text-[11.5px]"
                                :style="`--i: ${i}`"
                            >
                                <span class="text-dimmed text-[10.5px]">
                                    {{ String(i).padStart(2, "0") }}:{{
                                        String((i * 7) % 60).padStart(2, "0")
                                    }}
                                </span>
                                <span class="text-error font-semibold">×</span>
                                <span class="text-default truncate">{{ item.d }}</span>
                                <span
                                    class="border-error/25 bg-error/5 text-error/85 rounded-sm border px-1.5 py-0.5 text-[9.5px] font-medium tracking-[0.12em] uppercase"
                                >
                                    {{ item.t }}
                                </span>
                            </li>
                        </ul>
                        <div
                            class="from-default pointer-events-none absolute right-0 bottom-0 left-0 h-14 bg-gradient-to-t to-transparent"
                        ></div>
                    </div>
                </div>

                <div class="flex items-center gap-7 px-1.5">
                    <div class="flex flex-col gap-1">
                        <span
                            class="text-highlighted font-mono text-[22px] leading-none font-medium tracking-tight"
                        >
                            10M<span class="text-muted ml-0.5 text-[14px]">+</span>
                        </span>
                        <span
                            class="text-dimmed font-mono text-[9.5px] tracking-[0.08em] uppercase"
                        >
                            tracked domains
                        </span>
                    </div>
                    <span class="bg-(--ui-border) h-7 w-px" aria-hidden="true"></span>
                    <div class="flex flex-col gap-1">
                        <span
                            class="text-highlighted font-mono text-[22px] leading-none font-medium tracking-tight"
                        >
                            0.1<span class="text-muted ml-0.5 text-[14px]">%</span>
                        </span>
                        <span
                            class="text-dimmed font-mono text-[9.5px] tracking-[0.08em] uppercase"
                        >
                            false-positive rate
                        </span>
                    </div>
                    <span class="bg-(--ui-border) h-7 w-px" aria-hidden="true"></span>
                    <div class="flex flex-col gap-1">
                        <span
                            class="text-highlighted font-mono text-[22px] leading-none font-medium tracking-tight"
                        >
                            :53<span class="text-muted ml-0.5 text-[14px]">/udp</span>
                        </span>
                        <span
                            class="text-dimmed font-mono text-[9.5px] tracking-[0.08em] uppercase"
                        >
                            listening
                        </span>
                    </div>
                </div>
            </aside>

            <main class="flex justify-center">
                <div
                    class="form-card border-default bg-elevated/60 relative w-full max-w-md rounded-xl border p-8 backdrop-blur sm:p-10"
                >
                    <span class="form-corner tl" aria-hidden="true"></span>
                    <span class="form-corner tr" aria-hidden="true"></span>
                    <span class="form-corner bl" aria-hidden="true"></span>
                    <span class="form-corner br" aria-hidden="true"></span>

                    <div class="mb-7">
                        <span
                            class="text-primary mb-4 inline-block font-mono text-[11px] tracking-[0.1em]"
                        >
                            <span class="text-muted mx-1">[</span>
                            access required
                            <span class="text-muted mx-1">]</span>
                        </span>
                        <h1
                            class="text-highlighted text-4xl leading-none font-medium tracking-tight"
                        >
                            Authenticate<span class="text-primary cursor-blink">_</span>
                        </h1>
                        <p class="text-muted mt-3 max-w-[34ch] text-sm leading-relaxed">
                            Sign in to manage filtering rules, inspect query traffic, and tune the
                            sinkhole.
                        </p>
                    </div>

                    <UForm
                        :schema="schema"
                        :state="state"
                        class="flex flex-col gap-4"
                        @submit="onSubmit"
                    >
                        <UFormField name="login">
                            <template #label>
                                <span
                                    class="text-muted font-mono text-[10.5px] font-medium tracking-[0.16em]"
                                >
                                    <span class="text-primary mr-1.5">→</span> LOGIN
                                </span>
                            </template>
                            <UInput
                                v-model="state.login"
                                placeholder="admin"
                                size="xl"
                                autocomplete="username"
                                class="w-full"
                                :ui="{ base: 'font-mono' }"
                            />
                        </UFormField>

                        <UFormField name="password">
                            <template #label>
                                <span
                                    class="text-muted font-mono text-[10.5px] font-medium tracking-[0.16em]"
                                >
                                    <span class="text-primary mr-1.5">→</span> PASSWORD
                                </span>
                            </template>
                            <UInput
                                v-model="state.password"
                                type="password"
                                placeholder="••••••••"
                                size="xl"
                                autocomplete="current-password"
                                class="w-full"
                                :ui="{ base: 'font-mono' }"
                            />
                        </UFormField>

                        <UButton
                            type="submit"
                            color="primary"
                            size="xl"
                            block
                            :loading="isSubmitting"
                            trailing-icon="i-lucide-arrow-right"
                            class="mt-2 font-mono tracking-[0.18em]"
                        >
                            {{ isSubmitting ? "VERIFYING" : "AUTHENTICATE" }}
                        </UButton>
                    </UForm>

                    <div
                        class="border-default text-dimmed mt-5 flex items-center gap-2.5 border-t border-dashed pt-4 font-mono text-[10.5px] tracking-wide"
                    >
                        <span class="text-primary">↳</span>
                        <span>Session secured with encrypted cookies</span>
                    </div>
                </div>
            </main>
        </div>

        <footer
            class="border-default text-dimmed relative z-10 flex items-center gap-3 border-t px-6 py-3 font-mono text-[10.5px] tracking-wider sm:px-8"
        >
            <span>v1.0</span>
        </footer>
    </div>
</template>

<style scoped>
.bg-grid {
    background-image:
        linear-gradient(var(--ui-border) 1px, transparent 1px),
        linear-gradient(90deg, var(--ui-border) 1px, transparent 1px);
    background-size: 48px 48px;
    -webkit-mask-image: radial-gradient(ellipse 70% 50% at 50% 50%, #000 25%, transparent 100%);
    mask-image: radial-gradient(ellipse 70% 50% at 50% 50%, #000 25%, transparent 100%);
    opacity: 0.55;
}

.bg-glow {
    top: -8%;
    left: 32%;
    width: 880px;
    height: 880px;
    background: radial-gradient(
        circle,
        color-mix(in oklch, var(--ui-primary) 14%, transparent) 0%,
        transparent 65%
    );
    filter: blur(40px);
    animation: float-glow 22s ease-in-out infinite;
}

@keyframes float-glow {
    0%,
    100% {
        transform: translate(0, 0);
    }
    50% {
        transform: translate(-120px, 60px);
    }
}

.form-card {
    animation: rise 700ms cubic-bezier(0.2, 0.8, 0.2, 1) 100ms both;
}

.visual {
    animation: rise 700ms cubic-bezier(0.2, 0.8, 0.2, 1) both;
}

@keyframes rise {
    from {
        opacity: 0;
        transform: translateY(14px);
    }
    to {
        opacity: 1;
        transform: translateY(0);
    }
}

.form-corner {
    position: absolute;
    width: 14px;
    height: 14px;
    border-color: var(--ui-primary);
    pointer-events: none;
}
.form-corner.tl {
    top: -1px;
    left: -1px;
    border-top: 1px solid;
    border-left: 1px solid;
    border-top-left-radius: var(--ui-radius);
}
.form-corner.tr {
    top: -1px;
    right: -1px;
    border-top: 1px solid;
    border-right: 1px solid;
    border-top-right-radius: var(--ui-radius);
}
.form-corner.bl {
    bottom: -1px;
    left: -1px;
    border-bottom: 1px solid;
    border-left: 1px solid;
    border-bottom-left-radius: var(--ui-radius);
}
.form-corner.br {
    bottom: -1px;
    right: -1px;
    border-bottom: 1px solid;
    border-right: 1px solid;
    border-bottom-right-radius: var(--ui-radius);
}

.cursor-blink {
    animation: blink 1.05s steps(1, end) infinite;
    font-weight: 300;
    margin-left: 2px;
}

@keyframes blink {
    50% {
        opacity: 0;
    }
}

.stream-item {
    opacity: 0;
    animation: stream-in 600ms ease-out both;
    animation-delay: calc(var(--i) * 80ms + 300ms);
}

@keyframes stream-in {
    from {
        opacity: 0;
        transform: translateX(-10px);
    }
    to {
        opacity: 1;
        transform: translateX(0);
    }
}

.pulse-node {
    transform-origin: 184px 65px;
    animation: node-pulse 2s ease-in-out infinite;
}

@keyframes node-pulse {
    0%,
    100% {
        opacity: 0.55;
    }
    50% {
        opacity: 1;
    }
}

/* SVG color helpers — bound to Nuxt UI CSS variables so light/dark themes follow */
.svg-stroke-primary {
    stroke: var(--ui-primary);
    opacity: 0.7;
}
.svg-fill-primary {
    fill: var(--ui-primary);
}
.svg-fill-primary-soft {
    fill: color-mix(in oklch, var(--ui-primary) 8%, transparent);
}
.svg-text-primary {
    fill: var(--ui-primary);
    opacity: 0.9;
}

.svg-stroke-success {
    stroke: var(--ui-success);
    opacity: 0.65;
}
.svg-fill-success-soft {
    fill: color-mix(in oklch, var(--ui-success) 10%, transparent);
}
.svg-text-success {
    fill: var(--ui-success);
    opacity: 0.9;
}

.svg-stroke-error {
    stroke: var(--ui-error);
    opacity: 0.65;
}
.svg-fill-error {
    fill: var(--ui-error);
}
.svg-fill-error-soft {
    fill: color-mix(in oklch, var(--ui-error) 10%, transparent);
}
.svg-text-error {
    fill: var(--ui-error);
    opacity: 0.9;
}

.svg-stroke-muted {
    stroke: var(--ui-text-muted);
    opacity: 0.55;
}
.svg-stroke-faint {
    stroke: var(--ui-border);
}
.svg-text-dimmed {
    fill: var(--ui-text-dimmed);
}
</style>
