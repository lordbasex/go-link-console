// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
// Terms of use, privacy policy and footer texts in English, the reference
// version (docs/legal.md). es and pt translate this same shape.

/** One section of a legal page: paragraphs, then an optional list. */
export interface LegalSection {
  readonly id: string;
  readonly title: string;
  readonly body: readonly string[];
  readonly list: readonly string[];
}

export interface LegalTexts {
  readonly updated: string;
  readonly prevails: string;
  readonly contact: string;
  readonly terms: { readonly title: string; readonly intro: string; readonly sections: readonly LegalSection[] };
  readonly privacy: { readonly title: string; readonly intro: string; readonly sections: readonly LegalSection[] };
  readonly accept: {
    readonly before: string;
    readonly terms: string;
    readonly and: string;
    readonly privacy: string;
    readonly after: string;
    readonly guestAfter: string;
    readonly required: string;
  };
  readonly footer: {
    readonly tagline: string;
    readonly product: string;
    readonly project: string;
    readonly legal: string;
    readonly source: string;
    readonly releases: string;
    readonly license: string;
    readonly notices: string;
    readonly legalDoc: string;
    readonly rights: string;
    readonly trademark: string;
    readonly version: string;
    readonly github: string;
    readonly noRoms: string;
  };
}

export const legalEn: LegalTexts = {
  updated: "Version of September 28, 2026",
  prevails: "This page is a translation. If it differs from the English version, the English version prevails.",
  contact: "Questions, requests and reports: lord.basex@gmail.com",
  terms: {
    title: "Terms of use",
    intro:
      "go-link is free, open-source, independent and non-profit software. A host runs the go-link app on their own computer, with their own game files, and invites friends who play from a browser. The project's website and signaling server only introduce the host and the guests to each other. By linking a device, joining a game or otherwise using go-link, you accept these terms. If you do not accept them, do not use go-link.",
    sections: [
      {
        id: "roms",
        title: "ROMs and copyrighted content: zero-content policy",
        body: [],
        list: [
          "go-link does not host, include, distribute, sell, link to or download ROMs, game images, artwork, thumbnails or system BIOS files, and it does not name or point to any site that offers them.",
          "go-link is a software utility and a streaming engine. Every game file stays on the host's own computer; no go-link server receives, stores or relays it.",
          "The host is solely responsible for the files they load. Hosts may only use files they have the legal right to use, such as backups of arcade boards or software they own. Unauthorized reproduction or streaming of copyrighted material is the exclusive responsibility of the host who does it.",
          "Streaming a game to guests may be a public performance or communication of that game under the laws of your country. The host is responsible for making sure they are allowed to do it.",
        ],
      },
      {
        id: "use",
        title: "Acceptable use",
        body: ["You may not use go-link, its website, its signaling server or its relay to:"],
        list: [
          "stream, share or send content that is illegal, infringing, pornographic, defamatory, harassing, hateful, threatening or malicious, through video, voice or chat;",
          "harass, impersonate or harm other people, or collect their data without their consent;",
          "attack, overload, probe or abuse the project's servers, for example using the relay as a general-purpose proxy or bypassing its limits;",
          "make money from the emulator core or from games.",
        ],
      },
      {
        id: "rooms",
        title: "Hosts and their rooms",
        body: [
          "The project may block connections that break these rules. Hosts are responsible for who they invite and for what happens in their rooms: they can remove people, turn the chat off and close rooms at any time.",
        ],
        list: [],
      },
      {
        id: "age",
        title: "Age",
        body: [
          "You must be at least 13 years old, or the minimum age required in your country to use online services without a parent's consent. Younger people may only use go-link with a parent or guardian who accepts these terms.",
        ],
        list: [],
      },
      {
        id: "warranty",
        title: "No warranty",
        body: [
          'go-link, the website, the signaling server and the relay are provided "as is" and "as available", without warranties of any kind, express or implied, including merchantability, fitness for a particular purpose, availability and non-infringement. The service may change, fail, lose connections, have latency or stop at any time, without notice.',
        ],
        list: [],
      },
      {
        id: "liability",
        title: "Limitation of liability",
        body: [
          "To the maximum extent permitted by law, the authors and contributors of go-link are not liable for any direct, indirect, incidental, special, consequential or punitive damages, or for any loss of data, games, saves or profit, arising from the use of, or inability to use, go-link or its services, or from content that hosts or guests load, stream or send.",
        ],
        list: [],
      },
      {
        id: "indemnity",
        title: "Indemnity",
        body: [
          "If your use of go-link breaks these terms or the law, for example by streaming files you had no right to use, you agree to hold the authors and contributors of go-link harmless from any claim that results from it.",
        ],
        list: [],
      },
      {
        id: "licenses",
        title: "Licenses",
        body: [],
        list: [
          "go-link's own source code is released under the MIT license.",
          "The third-party components included in go-link's app and website keep their own licenses, listed in THIRD_PARTY_NOTICES.md and shipped with every release.",
          "The emulator core is not part of go-link: the app downloads mame2003-plus from the libretro buildbot at the host's request. mame2003-plus is under the MAME non-commercial license, so using go-link with that core is for non-commercial use only: any commercial distribution or monetization of go-link together with that core is not allowed.",
        ],
      },
      {
        id: "trademarks",
        title: "Trademarks",
        body: [],
        list: [
          "MAME® is a registered trademark of Gregory Ember. go-link is not affiliated with, endorsed by or sponsored by MAMEdev, the MAME team or libretro. The name MAME is only used to describe what go-link is compatible with, and the MAME logo is not used.",
          "Company names, game titles, logos and characters that games may show are trademarks and property of their respective owners. go-link is independent and does not claim any affiliation with, or endorsement by, any of them.",
        ],
      },
      {
        id: "reports",
        title: "Reports and changes",
        body: [
          "go-link hosts no games or user content, so it cannot remove them. To report abuse of the project's servers, or anything else about these terms, write to lord.basex@gmail.com.",
          "These terms may change. The version date is at the top of this page, and the website asks you to accept a new version before linking a device or joining a game again.",
        ],
        list: [],
      },
    ],
  },
  privacy: {
    title: "Privacy policy",
    intro:
      "go-link is designed to collect as little as possible. It has no accounts and no advertising, and the website's fonts are served by the website itself. The rooms (play.go-link.org), the device and the Player app have no cookies, no analytics and no third-party trackers. Only if you accept the cookie notice, Google Analytics counts visits to this website (console.go-link.org) with its cookies; that data goes to Google, under its own privacy policy, and you can change your choice at any time in \"Cookie settings\".",
    sections: [
      {
        id: "p2p",
        title: "Games, voice and chat travel peer to peer",
        body: [],
        list: [
          "Video, game sound, voice, controls and chat travel directly between the host's app and each guest's browser over WebRTC, which is always encrypted (DTLS and SRTP).",
          "When a direct connection is not possible, that traffic goes through the project's relay (TURN). It stays encrypted end to end: the relay forwards packets it cannot read.",
          "The project's servers never record, store or process the video, sound, voice or chat of a game.",
        ],
      },
      {
        id: "signaling",
        title: "What the signaling server sees",
        body: ["To introduce the host and the guests, the signaling server processes, in memory only and while the connection lasts:"],
        list: [
          "the IP address of each connection, to connect it and to apply limits against abuse;",
          "random connection identifiers, pairing codes, room identifiers and invitations;",
          "the WebRTC negotiation (which includes network addresses) and the room PIN a guest types, which it relays without storing;",
          "the app's identifier and a hash of its secret, kept in memory for up to 30 days to protect the app's identity.",
        ],
      },
      {
        id: "logs",
        title: "Technical logs",
        body: [
          "None of the above is written to disk or kept after the signaling server restarts, and it does not log IP addresses. The relay keeps technical logs that can include IP addresses (for example, blocked connection attempts); they rotate automatically, are capped in size and are only used to run and protect the service. The website is served by a hosting provider that processes standard request data (IP address, browser, date) to deliver the pages.",
        ],
        list: [],
      },
      {
        id: "browser",
        title: "What your browser keeps",
        body: [
          "The website stores preferences and keys in your browser's local storage, never in cookies and never on our servers (the only cookies are Google Analytics', and only if you accept them): language, theme, the signaling server you chose, the link with your app, return passes for games you joined, the local panel key, view and sound settings, and the date you accepted these terms. The skin editor and Willy Maker keep your skins and games the same way, in local storage and in the browser's own database (IndexedDB), and they never leave your browser unless you download them. Clearing this site's data in your browser removes all of it.",
        ],
        list: [],
      },
      {
        id: "host",
        title: "What the host's app keeps",
        body: ["The host's app runs on the host's own computer, and the host controls its data. It keeps, on that computer only:"],
        list: [
          "the linked browsers (only a hash of each key) and the version of these terms each one accepted;",
          "the game history: each room that ran and, for each guest, the name they typed, the seats they played, the connection path and the IP address the app saw (empty when the guest came through the relay);",
          "the network report of each room: from the room's first start until it is deleted for good, a sample per second of how the game and each guest's connection behaved (round trip, lost packets, frozen picture, the timing of the controls, voice packets, the latency test) and a log of events (who joined and left, the name they used, the connection path and IP address, freezes); only the host sees it, and deleting the room for good or a factory reset deletes it;",
          "the last 50 chat messages of each room while it runs, in memory;",
          "recordings, only when the host records a game: the game's picture and sound and the voice of each player, in a file on the host's computer. Everyone in the room is told while it records (a REC badge and a message in the chat). A recording stops by itself after 2 hours or 2 GB and when the game is paused. Only the host can download or delete it; deleting a game from the history, clearing the history or a factory reset deletes its recordings.",
        ],
      },
      {
        id: "addresses",
        title: "Network addresses between players",
        body: [
          "In a peer-to-peer connection, the host's app and the guests see each other's network addresses; that is how WebRTC works. If you do not want to share your address with a host, do not join that host's games.",
        ],
        list: [],
      },
      {
        id: "rights",
        title: "Your rights and your own servers",
        body: [
          "Because go-link keeps no accounts and no personal data on its servers beyond what is described here, most requests are solved by clearing your browser's data or asking the host to clear their history. For any question or request about your data, write to lord.basex@gmail.com.",
          "Anyone can run their own signaling server. When you use one (the website shows it), its operator, not the go-link project, is responsible for it.",
        ],
        list: [],
      },
    ],
  },
  accept: {
    before: "I have read and accept the ",
    terms: "Terms of use",
    and: " and the ",
    privacy: "Privacy policy",
    after: ", and I will only use ROMs I have the legal right to use.",
    guestAfter: ".",
    required: "Accept the terms of use to continue.",
  },
  footer: {
    tagline: "Arcade games streamed from your computer to your friends' browsers. Free, open source and non-profit.",
    product: "Product",
    project: "Project",
    legal: "Legal",
    source: "Source code",
    releases: "Downloads",
    license: "MIT license",
    notices: "Third-party licenses",
    legalDoc: "Full legal text",
    rights: "go-link. Code released under the MIT license.",
    trademark:
      "MAME® is a registered trademark of Gregory Ember. go-link is not affiliated with or endorsed by MAMEdev or libretro. Game names and logos belong to their owners.",
    version: "Website version",
    github: "go-link on GitHub",
    noRoms: "go-link does not host, include or download any ROM, BIOS or game image.",
  },
};
