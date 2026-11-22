Add the benchmark-only “Remove an eligible rule during drafting” feature.
At an Endless draft boundary, the host may spend the one draft decision to
remove one eligible active evolution rule instead of selecting a new rule or
keeping the field. This is an additional deterministic choice at each draft
where such a rule exists; it does not depend on the offered new-rule cards.
An eligible target is a currently active rule instance introduced after the
match started, not a starting baseline or lobby-selected initial rule. Removal
must leave all required mechanics, dependent active rules, and the next hand
plan valid. Show only eligible removals in the draft controls. Use the typed
WebSocket command `remove_rule` with the existing `rule` and `boundaryId`
fields. Reject stale boundaries, non-hosts, missing or ineligible rules, and
repeat decisions without changing state. A valid removal retains field
history, removes exactly one active instance, increments revision once,
clears the pending draft, and advances to hand-complete exactly once. Keep
existing choose and keep choices working.
