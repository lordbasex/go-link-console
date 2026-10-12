# Legal: terms of use, privacy, copyright and licenses

Version **2026-10-11**. This English text is the reference; the website shows it in English, Spanish and Portuguese, and if a translation differs, this version prevails. The same texts are published at [console.go-link.org/terms](https://console.go-link.org/terms) and [console.go-link.org/privacy](https://console.go-link.org/privacy).

This document is not legal advice. It describes how the project works and the conditions under which it is offered.

## 1. What go-link Console is

go-link Console is free, open-source software. A person (the **host**) runs the go-link Console app (the **device**) on their own computer, with their own game files, and invites friends (**guests**) who play from a web browser. The project also runs a website (console.go-link.org, with the rooms at play.go-link.org) and a signaling server (signal.go-link.org) that only introduce the host and the guests to each other.

go-link Console is independent and non-profit. It has no accounts, no payments, no advertising and asks for no donations.

## 2. Terms of use

By linking a device, joining a game or otherwise using go-link Console, you accept these terms. If you do not accept them, do not use go-link Console.

### 2.1 ROMs and copyrighted content: zero-content policy

- go-link Console does **not** host, include, distribute, sell, link to or download ROMs, game images, artwork, thumbnails or system BIOS files, and it does not name or point to any site that offers them.
- go-link Console is a software utility and a streaming engine. Every game file stays on the host's own computer; no go-link Console server receives, stores or relays it.
- **The host is solely responsible** for the files they load. Hosts may only use files they have the legal right to use, for example backups of arcade boards or software they own, or files whose rights holder allows it. Unauthorized reproduction or streaming of copyrighted material is the exclusive responsibility of the host who does it.
- Streaming a game to guests may be a public performance or communication of that game under the laws of your country. The host is responsible for making sure they are allowed to do it.

### 2.2 Acceptable use

You may not use go-link Console, its website, its signaling server or its relay to:

- stream, share or send content that is illegal, infringing, pornographic, defamatory, harassing, hateful, threatening or malicious, through video, voice or chat;
- harass, impersonate or harm other people, or collect their data without their consent;
- attack, overload, probe or abuse the project's servers (for example, using the TURN relay as a general-purpose proxy or bypassing rate limits);
- make money from the emulator core or from games (see section 4).

The project may block connections that break these rules. Hosts are responsible for who they invite and for what happens in their rooms; they can remove people, turn the chat off and close rooms at any time.

### 2.3 Age

You must be at least 13 years old, or the minimum age required in your country to use online services without a parent's consent, to use go-link Console. Younger people may only use it with a parent or guardian who accepts these terms.

### 2.4 No warranty

go-link Console, the website, the signaling server and the relay are provided **"as is" and "as available"**, without warranties of any kind, express or implied, including merchantability, fitness for a particular purpose, availability and non-infringement. The service may change, fail, lose connections, have latency or stop at any time, without notice.

### 2.5 Limitation of liability

To the maximum extent permitted by law, the authors and contributors of go-link Console are not liable for any direct, indirect, incidental, special, consequential or punitive damages, or for any loss of data, games, saves or profit, arising from the use of, or inability to use, go-link Console or its services, or from content that hosts or guests load, stream or send.

### 2.6 Indemnity

If your use of go-link Console breaks these terms or the law (for example, by streaming files you had no right to use), you agree to hold the authors and contributors of go-link Console harmless from any claim that results from it.

### 2.7 Reports

go-link Console hosts no games or user content, so it cannot remove them. To report abuse of the project's servers, or anything else about these terms, write to lord.basex@gmail.com.

### 2.8 Changes

These terms may change. The version date is at the top; the website asks you to accept a new version before linking a device or joining a game again.

## 3. Privacy policy

go-link Console is designed to collect as little as possible. It has no accounts, no cookies, no analytics, no advertising and no third-party trackers. The website's fonts are served by the website itself.

### 3.1 Games, voice and chat travel peer to peer

- Video, game sound, voice, controls and chat travel **directly between the host's device and each guest's browser** over WebRTC, which is always encrypted (DTLS and SRTP).
- When a direct connection is not possible, that traffic goes through the project's **TURN relay**. It stays encrypted end to end: the relay forwards packets it cannot read.
- The project's servers **never record, store or process** the video, sound, voice or chat of a game.

### 3.2 What the signaling server sees

To introduce the host and the guests, the signaling server processes, **in memory only** and while the connection lasts:

- the IP address of each connection (to connect it and to apply rate limits against abuse);
- random connection identifiers, pairing codes, room identifiers and invitations;
- the WebRTC negotiation (offers, answers and network candidates, which include IP addresses), and the room PIN typed by a guest, which it relays without storing;
- the device's identifier and a hash of its secret (kept in memory for up to 30 days, to protect the device's identity).

None of this is written to disk or kept after the process restarts. The signaling server does not log IP addresses.

### 3.3 Technical logs

- The **TURN relay** keeps technical logs that can include IP addresses (for example, blocked connection attempts). They are rotated automatically and capped in size, and are only used to run and protect the service.
- The website is served by a hosting provider that processes standard request data (IP address, browser, date) to deliver the pages.

### 3.4 What your browser keeps

The website stores preferences and keys in your browser's `localStorage`, never in cookies and never on our servers: language, theme, the signaling server you chose, the link with your device (device id, link id and token), return passes for rooms you joined, the local panel token, views and sound settings, and the date you accepted these terms. The skin editor and Willy Maker keep the user's skins and games the same way, in `localStorage` and in the browser's IndexedDB, and they never leave the browser unless the user downloads them. Clearing your browser's site data removes all of it.

### 3.5 What the host's device keeps

The host's device runs on the host's computer, and the host controls its data. It keeps, on that computer only:

- linked browsers (only a hash of each token);
- the game history: each room that ran, and for each guest the name they typed, the seats they played, the connection path and the IP address the device saw (empty when the guest came through the relay);
- the network report of each room: from the room's first start until it is deleted for good, a sample per second of how the game and each guest's connection behaved (round trip, lost packets, frozen picture, the timing of the controls, voice packets, the latency test) and a log of events (who joined and left, the name they used, the connection path and IP address, freezes); only the host sees it, and deleting the room for good or a factory reset deletes it;
- the last 50 chat messages of each room while it runs (in memory);
- recordings, only when the host records a game: the game's picture and sound and the voice of each player, in a file on the host's computer. Everyone in the room is told while it records (a REC badge and a message in the chat). A recording stops by itself after 2 hours or 2 GB and when the game is paused. Only the host can download or delete it; deleting a game from the history, clearing the history or a factory reset deletes its recordings.

In a peer-to-peer connection the host's device and the guests see each other's network addresses; that is how WebRTC works. Guests who do not want to share their address with a host should not join that host's games.

### 3.6 Your rights

Because go-link Console keeps no accounts and no personal data on its servers beyond what is described above, most requests are solved by clearing your browser data or asking the host to clear their history. For any question or request about your data, write to lord.basex@gmail.com.

### 3.7 Self-hosted servers

Anyone can run their own signaling server. When you use one (the website shows it), its operator, not the go-link Console project, is responsible for it.

## 4. Licenses

- **go-link Console's own source code** is released under the [MIT license](../LICENSE).
- **Third-party components** included in go-link Console's binaries and website (Go modules, libvpx, libopus, fonts, npm packages) keep their own licenses, reproduced in [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) and shipped with every release.
- **The emulator core is not part of go-link Console.** The device downloads mame2003-plus from the libretro buildbot at the host's request. mame2003-plus is under the MAME non-commercial license: it may not be sold or used for commercial purposes. Using go-link Console together with mame2003-plus is therefore **for non-commercial use only**; any commercial distribution or monetization of go-link Console with that core is not allowed by the core's license.
- The optional patches in `cores/mame2003-plus/` modify mame2003-plus and fall under its license.

## 5. Trademarks

- MAME® is a registered trademark of Gregory Ember. go-link Console is not affiliated with, endorsed by or sponsored by MAMEdev, the MAME team or libretro. The name MAME is only used to describe what go-link Console is compatible with; go-link Console does not use the MAME logo.
- Company names, game titles, logos and characters that games may show are trademarks and property of their respective owners. go-link Console is independent and does not claim any affiliation with, or endorsement by, any of them.

## 6. Contact

lord.basex@gmail.com
