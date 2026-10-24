# Monetization: every way, per platform

Status: examination of 2026-10-23, written at the founder's request. Nothing here is built except the entitlement layer it will plug into
(`internal/entitlements`, `internal/progress`). Nothing is decided: the decisions are listed at the end.

Founder's policy: the beta is free; after the beta the app is freemium; ads, sponsors, in-game features and unlocking philosophers are all open; low cost, many options; the audience is adults and
older teenagers (16+), not children.

**How reliable the numbers are.** Fees and store rules change. The figures below come from web searches on 2026-10-23, mostly from third-party articles that sometimes disagree; each platform's
own pricing and policy page must be read before any commitment. Items marked (verify) were not confirmed in any source found.

## 1. What can be sold or earned (the catalog)

| # | Option | What the person gets | Marginal cost to us | Notes |
|---|---|---|---|---|
| A | **Meet a philosopher now** (one-time, per philosopher or as a bundle) | skips the earning of PLAN 16; the earned route stays free | none | the cleanest product; the unlock rule already exists |
| B | **Supporter pack** (one-time) | no sponsor card or ads, themes, a quiet mention in About | none | the standard "remove ads" product, in the founder's tone: patron, not VIP |
| C | **Features with no running cost** | themes, a larger memory (the cap is 500 entries per philosopher), more export options, search of old conversations, a second profile | none | everything that runs on the device is free for us to give; do not sell features whose cost is the user's own key (suggestions, conversation length): that would be an artificial limit |
| D | **Subscription** (supporter, yearly) | the same as B, renewing | none | low fit: the person already pays the AI provider; allowed, not recommended |
| E | **Tip** (consumable) | nothing but thanks | none | inside Android it must go through Play Billing |
| F | **Ads** | nothing | an ad SDK (privacy, consent, size) | never inside a chat; see section 4 |
| G | **Rewarded ad** (optional) | a short pass (for example a philosopher for a day) | an ad SDK | Play has a product type for it; see section 4 |
| H | **Sponsor card** (sold directly) | a labelled card on the home screen, no SDK, no tracking | none | what Moly planned; small but fits the privacy promise |
| I | **Donations / memberships** (GitHub Sponsors, Ko-fi, Patreon) | nothing, or early builds | none | outside the stores; fine on desktop; inside Android see E |
| J | **Pay what you want** (itch.io) | the desktop app | none | good for the first desktop release |
| K | **Hosted AI credits** (we pay the model, the person pays us) | no key needed | high, plus a server | the largest option and the one that changes the product (needs a server we do not have, accounts, abuse control, and our own responsibility for what the model says); later, if ever |
| L | **Education or institutional licences** | classroom or library use | none | the audience is not children, but this needs its own age and privacy work; only a note |

Never: randomized purchases (loot boxes need disclosed odds in Play, and they do not suit a philosopher), pressure or countdown tricks, anything that switches the safety stage off (PLAN 11), anything that
sells the person's words. Wording follows the rule that nothing sounds like a sport or a game mechanic (see memory: philosopher voice in the UI).

## 2. Per platform

### Android (Google Play): the only store route for the phone
- **Developer account:** a one-time registration fee (about 25 USD, verify) and the identity check already under way.
- **Fees (third-party reports of the 2026 rules, verify in Play Console):** since the settlement in March 2026 the service fee on subscriptions is reported as 10% plus 5% for using Play Billing, 15% in all;
  for one-time in-app purchases 10% plus 5% on the first million of yearly revenue (sources disagree here: some say 15% effective, some 20%), and 20% to 25% plus 5% above it. Alternative billing and
  links to a payment on the developer's own website are reported as allowed in the US, UK and EEA at slightly lower fees. Google's own 15% programs exist for small developers.
- **Fits:** A, B, C, D, E (Play Billing), F and G (an ad SDK in the Android layer, outside the web page), H.
- **Rules that matter:**
  - **AI-generated content policy:** a chatbot is covered; the app must let the person report offensive AI output without leaving the app, and the reports must be used. **We do not have a report button yet; it is required before listing.**
  - **Target audience:** declare 16+ (no Families-program duties, no children's ad restrictions); misdeclaring risks removal.
  - **Randomized items:** the odds must be disclosed; we sell none.
  - **Rewarded ads as products:** Play documents a rewarded product type (the older pages describe a superseded API; check the current Billing Library).
- **Entitlement:** the plugin asks Play for the person's purchases at each start (restore and refunds come for free), the engine treats a purchased product as an unlock alongside earning. The check is on the
  device; a determined user can bypass it, accepted (PLAN 15).

### Windows
- **Microsoft Store:** a developer registration fee (verify). A reported commission of 12% for games and 15% for apps if Microsoft's checkout is used, and 100% to the developer for apps with their own checkout
  (one undated article; verify). Needs a packaged (MSIX) build; the app is not packaged yet, and nothing has been run on Windows.
- **Steam:** 100 USD per app (recoupable once the app earns 1,000 USD), 30% standard share. AI rules (2024): live-generated content is disclosed on the store page with a description of the guardrails, and players can
  report illegal output from the overlay; only live-generated adult sexual content is barred. A fit for A, B, C, J-style pricing, and sales and bundles.
- **itch.io:** an adjustable share, 10% by default, plus the payment processor (about 2.9% plus 30 cents); pay-what-you-want and keys. The cheapest first route.
- **Direct sale through a merchant of record:** Paddle 5% plus 50 cents; Lemon Squeezy 5% plus 50 cents with add-ons (its future after the Stripe announcement is unclear); Polar from 5% plus 50 cents on its free tier,
  lower on paid tiers; Creem is reported cheapest (all verify). They carry the world's sales taxes and VAT, which a solo seller cannot easily do. Licence keys: the app can check a key with the provider's API once, or offline against a
  signature of ours (see section 5). **The founder's earlier identity trouble was with Lemon Squeezy (an old-type ID card); each of these asks for identity.**
- **Fits:** A, B, C, I, J, K (later); F is not recommended on desktop.

### Linux
- No store with the reach of the others. **Fits:** itch.io, Steam (it runs on Linux), direct sale with a licence key, donations (I). Packaging (AppImage or Flatpak) is not done.

### macOS
- **Mac App Store:** reported 15% commission under the Small Business Program (below 1 million USD of yearly proceeds), 30% above; the yearly developer membership (about 99 USD, verify). Needs a sandboxed, signed,
  notarized build; macOS is not built or run at all yet, so this is the furthest option.
- **Outside the store:** itch.io, direct sale, donations, as for Linux.

### Web and browser extension (later, optional)
- The Chrome Web Store has no payments of its own (PLAN 1): direct sale with a licence key, donations, sponsors. Not planned.

## 3. Combinations (what goes together)

| Package | Android | Windows | macOS | Linux | Comment |
|---|---|---|---|---|---|
| **Quiet** (A, B, C, H, I) | Play one-time products + sponsor card | itch.io (pay what you want) + Steam later | itch.io | itch.io | no SDK, no tracking, smallest revenue, fits the promise best |
| **Quiet plus tips** (E) | tip as a Play product | donation links | donation links | donation links | add on top of any package |
| **Freemium with ads** (A, B, F, G) | Quiet + ads removed by B + optional rewarded pass | none (desktop has no good ad route) | none | none | needs consent screens, the data-safety form, one SDK; more revenue per free user, weaker privacy story |
| **Subscription** (D) | Play subscription | MoR subscription | none | MoR subscription | not recommended while the person already pays the AI provider |
| **Hosted credits** (K) | Play consumables as credits | MoR credits | same | same | needs a server; separate decision |

## 4. Ads and sponsors: the product rules they meet
- **No ads inside a chat, ever** (PLAN 1, Moly's rule), and none anywhere on a screen shown after a crisis or while a check-in is pending. Ads may only sit on the home screen or between sessions.
- **An ad network is a third party in the person's phone.** It collects the advertising ID and talks to its own servers. It never sees the chat (the page and the engine are separate from the SDK), but it
  contradicts the quiet promise unless it is explained and consented to. Needs: a consent screen where the law asks for it (the EEA), the Data Safety form in Play, the age-of-consent tagging the networks provide, and review of what ad categories can appear next to a philosophical app.
  (I found no source on how AdMob treats AI chatbot apps specifically; verify.)
- **The sponsor card avoids all of that:** a labelled static card sold directly, loaded from the app's own files, no tracking. It earns little but costs nothing and breaks no promise.
- **Rewarded ads** are optional by nature. The most natural reward is a time-limited pass to a philosopher, never a conversation limit lifted.

## 5. What it takes to build (per option)
- **Always:** `entitlements.Any(sources...)` (earned OR purchased OR the beta bypass, which also covers purchases), a product table (id, what it unlocks), a "Supporter" screen worded like the rest of the app, and the Memory and export code
  untouched. A refund or a lapsed subscription removes the entitlement at the next start.
- **Play:** the Billing Library in the Capacitor plugin, product ids in Play Console, restore on start; a **"Report this reply"** action in the chat (engine route plus screen), required by the AI-content policy.
- **Desktop licence key:** either the provider's online validation once (no server of ours), or an offline check of a key signed by us (the public key is in the engine, the private key stays with the founder; a tiny script or the
  provider's webhook issues keys). A key can be shared; accepted at these amounts.
- **Ads:** a native SDK in the Android layer only (never in the web page, whose security policy forbids any outside address), consent, no ad when the engine reports a safety state.
- **Documents:** THREAT_MODEL (purchase checks are local and bypassable), a privacy page that names every third party in use, store listings.

## 6. Decisions
**Decided by the founder (2026-10-23, later):** after the beta every option is open to the person, wherever the release platform can offer it: a totally free use with ads, small payments everywhere, tips paid with money or by watching an ad, philosophers unlocked by earning, buying or a pass, supporter products, sponsors, and GitHub Sponsors. The beta stays entirely free (the bypass in `internal/entitlements`). Prices belong to the platform that sells each product (a store price, an itch.io page), so none is fixed here.

**Built the same day (platform-neutral):** `internal/catalog` (what each product unlocks: `philosopher.<id>`, `philosophers.all`, `supporter`, and the time-limited `pass.<id>`), a `grants` table, `entitlements.Any` (earning, buying and passes are separate sources of one answer), `PUT /api/entitlements/grants` (a source's standing products, replaced as a whole so that a refund removes one), `POST /api/entitlements/pass` (an expiring pass, at most 72 hours, from an ad), `GET /api/entitlements`, and the **"Report this reply"** action Play's AI-content policy requires (the reply is taken out of the conversation, the summary and what the models read, and the person may keep a copy of the report to send to the developer; nothing is sent from the app).

**Still to decide or to build per platform:** the Play Billing and rewarded-ad code in the Android layer (needs Play Console products and a build uploaded to test), the consent screens and the privacy page naming every third party, the supporter benefits (themes and the like), the sponsor card, GitHub Sponsors and tip links (the shell needs a way to open an address in the browser), desktop routes (itch.io, Steam, a merchant of record), the contact address reports are to be sent to, and the roster of "Support" screens per platform (a screen shows only what its platform can offer).

**Hosted credits** (option K), explained: today the person brings their own key and pays the AI provider directly. With hosted credits the person would pay us instead, and we would pay the provider on their behalf from a server of ours that holds our own key, meters each person's use and checks who they are. It would remove the biggest barrier (most people have no API key), but it needs a server and money up front, makes their words pass through us, makes us answerable for abuse, and is a different business. Not planned; the founder had not met the term.

## 7. Open questions still for the founder
1. (Decided 2026-10-24: both. One product per philosopher, `philosopher.diogenes`, `philosopher.plato`, `philosopher.aristotle`, and a bundle, `philosophers.all`. Socrates is open from the start and has no product. Each store lists the same ids; the bundle's price is set below the sum on each platform.)
2. The supporter benefits, in the house voice (no sport or game words).
3. (Decided 2026-10-24: the same address as Moly, Hermes_creative@proton.me, shown in the report copy and in About.)
4. Ads: which network, and which screens may carry them (never a chat, never after a crisis).

## Sources (searched 2026-10-23, third-party unless marked)
- Google Play fees and policy changes: appbot.co, pricepush.app, launchshots.app, bluethrone.io, coda.co (blogs); check Play Console.
- Google Play AI-Generated Content policy: support.google.com/googleplay/android-developer/answer/14094294 (Google).
- Steamworks fee documentation: partner.steamgames.com/doc/gettingstarted/appfee (Valve); AI disclosure coverage: techradar.com, videogameschronicle.com.
- itch.io payments: itch.io/docs/creators/payments (itch.io).
- Merchant-of-record comparisons: fungies.io, alexcloudstar.com, buildmvpfast.com, aliteq.com (blogs).
- Apple Small Business Program: developer.apple.com/app-store/small-business-program (Apple).
- Play Billing rewarded products: developer.android.com/google/play/billing/billing_rewarded_products (Google, older API).
- Donations platforms: wmtips.com, talks.co, schoolmaker.com (blogs).
- Microsoft Store: one TechCrunch article (undated in the result); no current Microsoft page was found.
