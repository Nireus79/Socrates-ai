# Google Play listing: Socrates (free beta)

Local draft, 2026-10-24, from the code and docs/PRIVACY.md. Play's forms change: check every answer against the console. CHECK marks my best reading, not verified. Wording keeps the house voice: no sport or game words, no taglines about the philosophers.

## Main store listing
**App name** (max 30): `Socrates: ask a philosopher`
**Short description** (max 80): `Talk with artificial Socrates, Diogenes, Plato and Aristotle. Chats stay on your phone.`

**Full description** (max 4000):

Ask anything. Socrates does not hand down answers: he asks, and you think. Diogenes, Plato and Aristotle each have their own way of talking with you.

The philosophers are artificial characters, imagined alive in today's world and made from what was written about them and after them. They are not the real people, and they say so.

HOW IT WORKS
• Choose a philosopher and write, in plain English.
• He answers briefly, often with a question, and holds you to your own words.
• Stuck? "Help me answer" offers a few ways to begin, and you may change them.
• Now and then a philosopher puts a question to you that he thinks is worth thinking all the way through. Think enough of them through with one philosopher, and you meet the next.
• Each philosopher keeps short notes about what you said, only for himself. You can read and delete every one, or switch the memory off.

PRIVATE BY DESIGN
• Your conversations stay on your phone, in an encrypted database. Android backup is off.
• No account, no analytics, no tracking. There is no Socrates server: your messages go from your phone to the AI provider you chose, with your own key.
• "Report this reply" takes a reply out of your conversation. Nothing is sent unless you send it.

YOU NEED YOUR OWN KEY
Socrates works with a Claude (Anthropic) or OpenAI API key that you paste in at setup. Your messages are sent to that provider under its terms.

GOOD TO KNOW
• For people aged 16 and over.
• The philosophers are not a medical, legal or crisis service. If you are thinking of hurting yourself, call your local emergency number or a crisis line.
• Beta: questions and problems to Hermes_creative@proton.me.
• Independent app, not made or endorsed by Anthropic or OpenAI.

**Category:** Education (alternative: Lifestyle). **Contact email:** Hermes_creative@proton.me.
**Privacy policy URL:** to publish from docs/PRIVACY.md (open). **Website:** open.
**Graphics:** icon `mobile/store/play-icon-512.png`, feature graphic `mobile/store/feature-graphic-1024x500.png` (both made); phone screenshots 2 to 8 (not made; take them from the finished app, in the cream palette).

## App content
- **Target audience:** 16 and over only (CHECK: Play's age bands are 13-15, 16-17, 18+; select 16-17 and 18+ only). Not a Families app.
- **Content rating (IARC):** CHECK. No violence, sexual content, gambling or drugs shown by the app, but it is an AI chat where users may raise sensitive subjects: answer the AI-generated-content question truthfully (yes). Expect a "Teen" or higher rating.
- **AI-generated content policy:** the in-app way to report offensive output exists ("Report this reply"). Describe it in the declaration.
- **Ads:** none in the beta (answer "no"). Change before any ad or sponsor card ships.
- **App access:** reviewers need an API key. Give instructions and a key with a small spend limit, revoked afterwards. This is the main practical blocker (founder's key, final stage only).
- **Data safety (CHECK):** collects/shares: the text of conversations is sent to the AI provider the user chose (data shared with a third party, for app functionality; the user configures it). No data is collected by the developer. Data is encrypted at rest; the user can delete all data in the app (no account, so no deletion URL needed; CHECK whether Play wants one).
- **Permissions:** INTERNET, foreground service (data sync) with the "running" notice, POST_NOTIFICATIONS. No storage, contacts, location, camera or microphone.
- **Purchases:** none in the beta. After the beta: one-time products per philosopher and a bundle (docs/MONETIZATION.md).
- **Target API level / 64-bit:** SDK 36, arm64-v8a (the engine library is arm64 and x86_64 for the emulator).
