// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Ports of the test stack, apart from the developer's own (signalhub 8090,
// web 5180, Willy Maker 5181, panel 7373) and from other projects (8080, 3478, 5173).
export const PORTS = { signal: 8191, web: 5191, maker: 5192, site: 5193, play: 5194, panel: 7391 };

/** Willy Maker's own site in the test stack. */
export const MAKER_URL = `http://localhost:${PORTS.maker}`;

/** The two builds of the website, each on its own origin: the landing (console.go-link.org) and the rooms (play.go-link.org). */
export const SITE_URL = `http://localhost:${PORTS.site}`;
export const PLAY_URL = `http://localhost:${PORTS.play}`;
