# Domain inspection: optional external API keys

The `/api/domain/inspect` endpoint (and the **Inspect** page in the UI) runs
a fan-out of independent checks against a single domain and aggregates a
verdict (`clean` / `unknown` / `suspicious` / `malicious`) plus a risk score
from 0 to 100. The checks fall into two groups:

- **Always on, no setup required:**
  - `local_stats` — what your own server already knows (block/allow lists,
    historical block-event counters).
  - `dns_resolve` — system resolver lookup for A/AAAA/MX/NS.
  - `rdap` — registration age via `rdap.org` (reduced to eTLD+1).
  - `crtsh` — Certificate Transparency history via `crt.sh`.

- **Optional, gated by an API key:**
  - `virustotal` — `DNS_FILTER_VT_KEY`
  - `urlscan` — `DNS_FILTER_URLSCAN_KEY`
  - `safe_browsing` — `DNS_FILTER_SAFE_BROWSING_KEY`

> **All three optional checks are exactly that — optional.** If a key is
> unset the corresponding check returns `status: skipped` (not `error`), and
> the summary verdict is computed from whatever checks did run. You can
> enable any combination of them, in any order, at any time — they are
> independent.

The rest of this document explains, **for each optional provider**, what it
adds to the inspection, whether/how much it costs, how to obtain a key, and
how to verify it is working.

---

## TL;DR

| Provider                                      | Env var                        | Cost (current free tier)               | Best at                                    |
| --------------------------------------------- | ------------------------------ | -------------------------------------- | ------------------------------------------ |
| [Google Safe Browsing v4][gsb]                | `DNS_FILTER_SAFE_BROWSING_KEY` | Free for **non-commercial** use        | High-precision malware/phishing verdicts   |
| [VirusTotal][vt]                              | `DNS_FILTER_VT_KEY`            | Free: 4 req/min, 500/day, 15.5k/month  | Aggregated AV-vendor verdicts + categories |
| [urlscan.io][urlscan]                         | `DNS_FILTER_URLSCAN_KEY`       | Free: ~1000 search req/day             | Recent real-world page renders, screenshots |

[gsb]: https://developers.google.com/safe-browsing/v4
[vt]: https://www.virustotal.com
[urlscan]: https://urlscan.io

After adding a key, **restart the backend** — config is loaded once at
startup via `sync.Once`, hot-reload doesn't pick up new env vars.

---

## 1. Google Safe Browsing — `DNS_FILTER_SAFE_BROWSING_KEY`

### Why enable it

Safe Browsing is the same list Chrome and Firefox use to show the red
"Deceptive site ahead" interstitial. The list is **conservative** — Google
only adds confirmed malware, phishing, or unwanted-software endpoints — so
the signal is very high-quality. In our scoring a single hit immediately
escalates the verdict to `malicious`. An empty 200 OK from Safe Browsing is
also a strong **clean** signal: Google actively saying "we know about this
domain and have nothing on it."

This is the check I'd enable first if I had to pick one.

### Cost

**Free** for non-commercial use. No credit card required.

- Personal / home / lab / hobby projects: free, just create a Google Cloud
  project, no billing setup needed.
- Quotas are generous: hundreds of requests per minute, tens of thousands
  per day per project. For "click Inspect" usage you will never feel them.
- **Commercial use** (selling this as a service, gating paying customers'
  traffic, etc.) is **not allowed** by the free Safe Browsing API ToS — you
  would need either Google's [Web Risk API][web-risk] (paid) or a separate
  signed agreement with Google.

[web-risk]: https://cloud.google.com/web-risk

### How to enable it

1. **Create a Google Cloud project** (skip if you already have one):
   1. Go to <https://console.cloud.google.com/>.
   2. Project selector (top bar) → **New Project**.
   3. Give it any name, e.g. `dns-filter-personal`. Do **not** attach a
      billing account — not needed for this API.

2. **Enable Safe Browsing API:**
   1. Side menu → **APIs & Services** → **Library**.
   2. Search for `Safe Browsing API` → open it → **Enable**.

3. **Create an API key:**
   1. Side menu → **APIs & Services** → **Credentials**.
   2. **Create credentials** → **API key**. Copy the value
      (starts with `AIzaSy...`, ~39 chars). Store it somewhere safe.
   3. **Click "Edit API key"** and apply two restrictions — leaked keys
      are otherwise free to abuse on your project:
      - **API restrictions** → **Restrict key** → tick only
        `Safe Browsing API`.
      - **Application restrictions** → **IP addresses** → add the public
        IP(s) of the host running `dns-filter`. Leave as `None` only for
        local development.

4. **Add the key to your `.env`:**
   ```env
   DNS_FILTER_SAFE_BROWSING_KEY=AIzaSy...paste_here
   ```

5. **Restart the backend** (`air` will do this automatically on the next
   file change, or run `go run main.go` again).

### How to verify

Google publishes deliberately-flagged test hostnames. Open the **Inspect**
page in the UI and try:

```
testsafebrowsing.appspot.com
```

Expected — the `safe_browsing` card shows `status: ok`,
`verdict: malicious`, with `threat_types` populated (typically
`SOCIAL_ENGINEERING`, `MALWARE`).

For a sanity check on the clean path, try `google.com` or `github.com`:
`status: ok`, `verdict: clean`, `matches: 0`.

If you see `status: skipped` with `DNS_FILTER_SAFE_BROWSING_KEY not set` —
the env var didn't make it into the process. Re-check `.env` and that the
backend was restarted.

---

## 2. VirusTotal — `DNS_FILTER_VT_KEY`

### Why enable it

VirusTotal aggregates the verdicts of ~90 antivirus engines and threat
intelligence vendors for a given domain. The `details` block returned by the
check includes how many vendors flag it as malicious vs. suspicious vs.
harmless, plus categories (advertising, malware, phishing, …) and tags.

Scoring:

- `malicious ≥ 3` engines → verdict `malicious`.
- `malicious ≥ 1` or `suspicious ≥ 2` → `suspicious`.
- Only harmless/undetected → `clean` (mild signal — VT seeing the domain
  and finding nothing).
- 404 from VT (domain it has never observed) → `unknown`.

VT is great for context ("which vendors agree, which disagree") even when
Safe Browsing already gave a verdict.

### Cost

**Free** with limits:

- Free public API: **4 requests/minute, 500/day, 15.5k/month**.
- Enough for ad-hoc human inspection; not enough for automated batch
  scanning of large block-list deltas.
- No credit card required.
- Paid Premium tier exists for higher throughput and richer endpoints.

### How to enable it

1. Sign up at <https://www.virustotal.com/gui/join-us>.
2. After login, open the user menu → **API key**. Copy the value
   (looks like a 64-char hex string).
3. Add to `.env`:
   ```env
   DNS_FILTER_VT_KEY=your_vt_key_here
   ```
4. Restart the backend.

### How to verify

Inspect a domain VT very likely knows about — e.g. `wikipedia.org`. The
`virustotal` card should show `known_to_vt: true`, plenty of `harmless`
votes, `malicious: 0`, and `verdict: clean`.

`skipped` with `DNS_FILTER_VT_KEY not set` means the env var didn't load.

---

## 3. urlscan.io — `DNS_FILTER_URLSCAN_KEY`

### Why enable it

urlscan.io is a community-driven service that scans URLs in a real headless
browser, capturing the DOM, screenshot, network requests, TLS chains, and
verdicts. We do **not** submit new scans from the inspect endpoint (that
would consume the submit quota and is asynchronous); instead we **search**
the public archive for prior scans of the domain.

Scoring:

- Any prior scan with `verdicts.overall.malicious: true` → `malicious`.
- `max_score ≥ 50` without an explicit malicious flag → `suspicious`.
- Scans exist and look clean → `clean` (real-world confirmation).
- No scans → `unknown`.

urlscan is especially useful for popular consumer-facing domains: someone
in the community has almost certainly scanned them already.

### Cost

**Free** with limits:

- Free tier: roughly **1000 search requests per day** (more than enough
  for human-driven inspection).
- Submitting new scans has a separate quota — we don't touch that.
- No credit card required.

### How to enable it

1. Sign up at <https://urlscan.io/user/signup>.
2. After login → **Settings** → **API**. Generate a key.
3. Add to `.env`:
   ```env
   DNS_FILTER_URLSCAN_KEY=your_urlscan_key_here
   ```
4. Restart the backend.

### How to verify

Inspect any popular domain — e.g. `cloudflare.com`. The `urlscan` card
should show `scans_found > 0`, `malicious_hits: 0`, and `verdict: clean`.

---

## Operational notes

### Where keys live

All three keys are read from environment variables at startup. In local
development they normally come from `.env` (see `.env.example` for the
template). `.env` is in `.gitignore`, so secrets do not leak into the
repository.

When deploying via Docker / systemd / Kubernetes, pass them through your
normal env-var mechanism (`docker compose --env-file …`, K8s `Secret`, etc.).

### Why a restart is required after editing keys

Config is loaded once at process startup via `sync.Once` in
`config/GetConfig()`. Changing `.env` while the server runs has no effect
until the process restarts. `air` will restart on the next Go file change;
otherwise just rerun `go run main.go` or restart the container.

### What happens when a key is wrong or revoked

The check returns `status: error` with the upstream HTTP code in the body
— for example `safe browsing http 403` for an over-quota / wrong-API Safe
Browsing key. The aggregated verdict still works from the remaining
checks; only the misconfigured one is marked failed in the UI.

### Privacy considerations

When you inspect a domain, that domain name is sent to every enabled
external service. If that is a concern (e.g. you're inspecting an internal
hostname), disable the external keys for that environment — the
always-on checks (`local_stats`, `dns_resolve`, `rdap`, `crtsh`) are
sufficient for many decisions and stay on your network (except crt.sh and
rdap.org, which receive the domain name as part of the lookup).
