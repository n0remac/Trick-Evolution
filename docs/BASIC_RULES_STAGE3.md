# Basic Play and Rule Field: Stage 3

Stage 3 places the public active rule field above the 3D table and keeps its card details synchronized with the Stage 1 resolution and Stage 2 Game Rules review. It adds no rule effects, evolution cards, or gameplay commands.

## Layout contract

The GoDom field has four ordered zones: **Hand**, **Play**, **Trick**, and **Score & Victory**. Desktop uses four columns; widths at or below 700 CSS pixels use two columns in the same reading order. Each zone keeps a text label and a border accent. Empty zones say **Base rules only** and contain no placeholder card. Initially, Play contains only Follow Suit and Trick contains only High Card Wins. Hidden baselines stay in Game Rules.

Each visible card is a semantic button with a playing-card proportion, category, title, and a short summary of its *currently effective* typed effects. The full-detail dialog lists those effects, prerequisite status, actual replaced rules, and any effect the card no longer controls. A partly replaced card is labelled as such. A superseded or prerequisite-inactive card leaves the field and remains available through the Changes history. If an inspected card changes during a live update, the detail dialog refreshes from the new projection or states that the card is no longer active; it never keeps old active wording.

Within each category, explicit replacement ancestry determines a stable display slot. A successor inherits its earliest same-category replaced slot; a partial successor appears before the ancestor's remaining effect card. Independent additions use their own sequence slot and wrap beside other cards. The four categories organize only presentation and do not change replacement mechanics. The default evolution deck remains empty; synthetic supported definitions exercise larger fields in tests.

The status cue stays above the rule field and pairs the latest event with the next turn or action. A reveal says which hand will first use the new rule. The Rules button and End Game control remain separate above it. The rule field scrolls vertically when it grows. A `ResizeObserver` measures the rendered HUD and reserves that height, plus a gap, from the canvas's usable viewport; the bottom also honors the safe-area inset. Resizing and rotation recompute the inset without remounting the canvas or changing the animation queue, sorted hand, selection lift, or drag handling.

## Projection and synchronization

The field uses `PublicView.ActiveField`, `Rules` history, and `Effective` effects. Card details and the Stage 2 review derive from the same projection. JavaScript handles local scrolling, dialog content freshness, and measured geometry; it does not resolve rules. Live screen swaps retain the Stage 2 review state and tab. Reconnect renders the current field from the server projection, while the existing presentation restore path avoids replaying earlier card landings. Private hands and future rule draw order are never used in field markup.

## Validation and limits

- Focused Go tests cover the exact two-card start, four category order and empty labels, typed effect details, private-card absence, sequential replacement position, hidden baseline replacement, independent additions, partial replacement, inactive dependency, synthetic multi-card Hand zone, deterministic reconnect order, and next-hand reveal wording.
- `scripts/test-release.sh` passed the focused Go packages, ten-hand HTTP/WebSocket fixtures, and root/browser-runner builds. `node --check gameui/game.js`, Go formatting, and `git diff --check` passed.
- The bounded `rules-smoke` browser mode now checks the field, details, Rules review, legal play, and measured HUD/table geometry at phone portrait, phone landscape, and desktop widths. One local 60-second attempt stopped before page creation because Chromium did not publish `DevToolsActivePort` within 30 seconds; its log was empty. Stage 2 attempts had also timed out in DevTools on this Pi. No viewport assertion completed, so visual, phone/touch, keyboard, and assistive-technology behavior still need a successful browser/manual pass on a less constrained machine. No repeated browser loops were run.

The release gate covers Go behavior and builds, not rendered pixel geometry. The UI geometry contract is encoded in CSS and measured by the browser smoke when Chromium is available.

## Later playtest layout

The compact horizontal rule strip in [Pitch scoring and pregame rules](PITCH_SCORING_AND_PREGAME.md) supersedes the two-column and four-column field layout described above. The field categories, detailed tap dialog, and active rule projection remain the same.
