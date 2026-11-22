# Fixed review rubric

Apply the same rubric and evaluator to both layouts of each task. Evaluate
observable behavior first; do not award or deduct points based on file names,
symbol names, or where the agent placed code. Do not expose this rubric or the
evaluator tests in candidate checkouts.

| Criterion | Points | Invite evidence | Removal evidence |
| --- | ---: | --- | --- |
| Core outcome | 4 | Guest joins the existing room and hand with a separate session; both players see each other. | An eligible active evolution rule can be removed during a draft; the draft settles exactly once. |
| Invalid actions | 2 | Invalid, expired, or full invites leave the room unchanged. | Non-host, stale, absent, starting, dependent-breaking, and repeat removals leave state unchanged. |
| Page interaction | 2 | Share and join flow is usable with independent browser sessions. | Eligible removal is visible and works from the draft control. |
| Regression safety | 2 | Solo Endless and regular room joins still work. | Choose, keep, rules, and next-hand behavior still work. |

Record a score from 0 to 10, evidence for each criterion, acceptance status,
elapsed time, and any browser check status. A failing acceptance command is
not automatically a zero score; inspect its failure details before review.
