# Offline regrade: invite historical 002

The original live result remains at `_generated/live-shakedown/invite-historical-002/result.json` (SHA-256 `380d3974a5ff38ea34123a769ff96259b4015ce4d1786409f62cb03ae31aa59a`). Its acceptance failed because the old evaluator required an `invite` field in the Endless start response. That field is not required by the prompt.

The saved candidate patch has SHA-256 `13eaba0f877824e0bd21f5796f05282397cfcb8d1d1a87d2f8c358a83388f7cf`, matching the original result's `candidate_sha256`. The revised invite evaluator had SHA-256 `d789ab455ddc41a9711d93bed6c912d7c50c99fe14c7a628897bc6e563eac661` when graded. The fake patch adapter applied that patch to a fresh checkout and graded it without a model call.

| Fixture | Revised acceptance | Evidence |
| --- | --- | --- |
| Known-good historical invite patch | Pass, exit 0 | `_generated/regrade/invite-historical-reference/result.json` |
| Saved candidate patch | Pass, exit 0 | `_generated/regrade/invite-historical-002/result.json` |
| Baseline with inert marker patch | Fail, exit 1 | `_generated/regrade/invite-historical-missing/result.json` |

The revised evaluator tests an invite returned at start or one issued from the live session. It checks two websocket views, private cards and authority, state continuity, invalid and wrong-room credentials, expiration or room closure, a full table, and ordinary room joins. This regrade did not run the separate Chromium browser smoke.

The regrade JSON and Markdown include a `cost` section sourced from the saved `turn.completed.usage`, with 1,356,986 input tokens, including 1,313,408 cached and 43,578 uncached, plus 11,406 output tokens. The 2,499 reasoning tokens are part of output, not an additional bucket. `actual_billed_api_usd` is `null` because authoritative billing data is unavailable. `estimated_api_usd` is also `null` because no explicit model rates were supplied. The offline regrade made no model API call.
