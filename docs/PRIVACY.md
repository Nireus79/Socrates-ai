# Privacy: what Socrates stores, where it goes, and how it is removed

Status: draft of 2026-10-24 for the beta. It is the source for the privacy page the stores ask for; the page itself is still to be published at a public address (open). It must be updated before any billing or ad code ships: those add third parties and each must be named here. Technical detail: [THREAT_MODEL.md](THREAT_MODEL.md).

## In plain words
- Your conversations, the notes the philosophers keep about you and your settings are stored **only on your own device**, encrypted. We (Hermes Software) run no server and receive nothing.
- To write a reply, the text of the conversation is sent to the **AI provider you chose** (Anthropic or OpenAI) with **your own key**. That provider's terms and privacy policy apply to what it receives. Nothing else leaves your device.
- There is no account, no analytics, no advertising and no tracking in the beta.
- The philosophers are artificial. They are not a medical, legal or crisis service. The app is for people aged 16 and over.

## What is stored, and where
- A database in the app's private folder (`~/.socrates` on a computer; the app's own storage on Android, with backup switched off). The sensitive fields (message text, titles, summaries, the notes in the memory, the provider key) are sealed one by one with AES-256-GCM. The key lives in the system's keychain (Android Keystore on a phone), or is sealed by a passphrase you choose.
- **Conversations**: messages, a title and a running summary per conversation.
- **Memory** (can be switched off in Settings): short notes each philosopher keeps for himself, with your own words as evidence: claims, the words you defined, tensions you saw, and a few facts about your life. He never keeps health, sexual life, money, legal trouble or named third parties by design (a model decides, so check the Memory screen). You can see and delete any note. Notes are per philosopher and are not shared between philosophers.
- **Progress**: how many questions you have thought through with each philosopher (no text).
- **Settings and purchases**: your provider choice, your key, whether memory is on, and later the products you bought or earned.
- **Reports**: "Report this reply" removes the reply from the conversation, from what the models read and from the summary. Nothing is sent. If you choose to copy the report, you send it yourself to Hermes_creative@proton.me.

## What is never stored or logged
- Your words, the philosophers' words and your key are never written to the log; a test starts the real program, runs a conversation and fails if any appears in the log or in plain text in any file.
- When a message is refused or is a crisis message, its words are not kept; only a marker that something was refused.
- Nothing is learned from a message while you may be hurting.

## Where it goes
Each turn makes a few short calls to your provider: a safety check of your message, the reply, a safety check of the reply, and afterwards (in the background) a title, a running summary and the notes for the memory; "Help me answer" asks for suggestions. The text of the conversation is in those calls. The provider may keep it under its own terms; use a key with a spending limit and read its policy.

## Your controls
- **Memory off** (Settings): no new notes, and the old ones are not told to the philosophers.
- **Delete a conversation**: if the philosopher learnt something in it, you choose whether he forgets that too (forgetting is the default).
- **Export / import** (Settings): one file with your conversations, notes and progress; never your key.
- **Delete everything** (Settings): removes every row, your key and your settings, and compacts the file.
- Uninstalling the app removes all of it on a phone.

## Known limits (see THREAT_MODEL.md)
Malware running as you can read what the app can read; a copy of an older database file is outside our control; progress and purchases are stored locally and can be edited by someone who controls their own device.

## When billing and ads arrive (not in the beta)
Each service must be named here with what it receives: the store (Google Play, Steam, itch.io or a merchant of record) for purchases, an ad network for rewarded ads (with the consent screen it requires), GitHub or a tip service when you open its link. No ad will ever appear in a conversation or after a crisis step.
