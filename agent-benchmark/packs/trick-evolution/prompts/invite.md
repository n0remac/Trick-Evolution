Implement “Join the exact Endless hand from an invite.” A player in a live
Endless run must be able to share an invitation link. Opening that link in a
separate browser session lets another named player take a vacant bot seat in
that same room and current hand. The guest gets a distinct private session;
both players see each other at the table. Preserve the hand's cards, turn,
trick, scores, and rules rather than making a new room or deal. A reconnect
token must never serve as the shared invitation authority. Reject expired,
invalid, and full-table invitations without changing the game. Keep existing
solo Endless play and ordinary room joins working. Implement the page
interaction as well as the server and domain behavior.
