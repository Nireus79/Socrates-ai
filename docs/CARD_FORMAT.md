# The card file: how to change the card from GitHub

The card at the foot of the screens is described by a data file in this repository, `site/cards.json`. Change the file, push it, and the apps show the new card within a day, with no new release. The file is DATA, never code: the apps check every field and drop anything that fails.

This file is read by **Socrates** only, from `https://raw.githubusercontent.com/Nireus79/Socrates-ai/master/site/cards.json` (the engine fetches it, never the page). Moly has its own file in its own repository, so the two apps can show different sponsors.


## What stays fixed (the apps draw it, the file cannot change it)
The label **Sponsored** on every card (decided 2026-11-08), the Hide button, that links open in the system browser, and that only https links are kept (no user name, no port). No scripts, no markup, nothing fetched from anyone but this file.

## The file
```json
{
  "version": 1,
  "entries": [ ... ],
  "cards":   [ ... ]
}
```
- `version` stays 1.
- `cards` is the current format (below). The apps use it when at least one card in it is valid.
- `entries` is the first format, which the apps already downloaded before the cards existed still read (they ignore `cards`). They **refuse an entry without a message**, so keep one entry with `kind`, `title`, `text` (1 to 280 characters), `marks: ["hermes"]`, `links` and `active`, written for them. New apps ignore it while a valid card exists, and use it as a credit row otherwise.
- At most 10 entries and 10 cards, the whole file at most 128 KB. A card that fails any check is dropped on its own.

## A card
| Field | Meaning |
|---|---|
| `id` | 1 to 64 characters, required |
| `active` | `true` or `false`, required (`false` retires the card) |
| `layout` | `credit-row`, `logo-center` or `logo-text`, required |
| `credit` | up to 40 characters; the words before the logo; required for `credit-row`, ignored by the others |
| `text` | up to 280 characters; required for `logo-text`, optional for `credit-row`, ignored by `logo-center` |
| `logo` | `{ "mark": "hermes" }` or `{ "image": "data:image/png;base64,...", "alt": "...", "tint": true }`; required for `logo-center` and `logo-text`, optional for `credit-row` |
| `buttons` | up to 4 of `{ "text": up to 30 characters, "url": "https://...", "primary": true }`; `primary` is drawn dark |
| `from`, `until` | optional first and last day shown, `YYYY-MM-DD` (UTC); a sponsor's slot ends by itself |

When several cards are active (and within their days) one is chosen by the day, so the card changes slowly and depends on nothing about the person.

### Layouts
- `credit-row`: the credit line with the logo on the left, the buttons on the right (below on a narrow screen and on a phone). This is the card of Hermes Software.
- `logo-center`: one big logo in the middle, the buttons under it. Nothing else.
- `logo-text`: the logo beside the message (above it on a narrow screen), the buttons last.

### The logo
- `mark: "hermes"` is the built-in Hermes Software mark, drawn in the colour of the theme.
- `image` is a PNG **inside the file** (a data address), so that nothing is fetched from a sponsor and the sponsor learns nothing about the person. PNG only (no SVG: it can carry scripts), at most **24 KB** and **1200 x 400 pixels**, checked by its real header, not by its name. `alt` (up to 100 characters) is required.
- `tint: true` is for black line art on a transparent ground: it turns light on the dark theme. Leave it off for a coloured logo.
- To make the address: `base64 -w0 logo.png`, then put `data:image/png;base64,` in front. Crush the PNG first (for example `pngquant`); a logo drawn at 88 pixels tall needs no more than about 300 x 100.

### Example: the Hermes card, and a sponsor's logo-only card
```json
{ "id": "created-by-hermes", "layout": "credit-row", "credit": "Created by", "logo": { "mark": "hermes" }, "active": true,
  "buttons": [ { "text": "Sponsor", "url": "https://github.com/sponsors/Nireus79", "primary": true },
               { "text": "GitHub", "url": "https://github.com/Nireus79" },
               { "text": "Hermes", "url": "https://hermesoftware.wordpress.com/" } ] }

{ "id": "acme-2026-12", "layout": "logo-center", "active": true, "from": "2026-12-01", "until": "2026-12-31",
  "logo": { "image": "data:image/png;base64,iVBOR...", "alt": "Acme" },
  "buttons": [ { "text": "Visit Acme", "url": "https://example.org/", "primary": true } ] }
```

## Before you push
Run `go test ./internal/cards/` in `engine/`: `TestTheShippedFeedIsValid` fails if `site/cards.json` is not a valid feed, holds no card, or holds no entry for the older apps. `make check` runs it too.
A third party's card makes the app carry the F-Droid `Ads` anti-feature (declare it in the F-Droid recipe before the first sponsor card), and the store listings should say a sponsor card may appear.

## Limits you should know
- The apps fetch the file at most once a day, so a change reaches a person within about a day of their next opening the app.
- Apps downloaded before the cards existed only show the entry of the first format, in their old layout; they never learn a new layout.
- A card with an image makes the file larger: keep sponsors' logos small.
