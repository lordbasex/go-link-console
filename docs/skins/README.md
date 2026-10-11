# Gamepad skins

A skin draws a console shell around the game on a phone: the plastic, the picture's bezel, the D-pad, the action buttons, Coin and the start buttons, and the room's menu capsule. **A skin is only data**: one JSON file, optionally in a folder with its background pictures. Both Player apps (iOS and Android) draw every skin with the same painter and the same placement math, so a new skin never needs new code. Skins are easy to write by hand, to draw in the [editor](#the-editor), to generate in bulk (for example with another AI given this page and [skin.schema.json](skin.schema.json)), and to share.

## This folder

| Path | What it is |
| --- | --- |
| `README.md` | This guide: how skins work and how to design one. |
| `skin.schema.json` | The format as a JSON Schema (draft 2020-12): any validator checks a skin file with it. |
| `screens.json` | The screens a skin is designed on and checked against: every iPhone from the SE and the 11 to the 17 Pro Max and the Air (phones with the same screen share one entry, for example "iPhone 14 Pro · 15 · 15 Pro · 16"), an iPad, and Android phones from 360 × 640 to 461 × 998 dp, a foldable and a tablet, in points or dp with their safe areas. The editor's phone menu and its checks read it; the apps' paste check keeps the same list (`SkinCheck.screens`, `SkinInstall.SCREENS`) and a test in each app compares it with this file. |
| `builtin/skin-<id>.json` | The built-in skins (Violet, Red, Green, Blue, Smoke, Orange). **The apps build from these files**: the iOS project copies them into the app and the Android build takes the folder as assets, so a change here ships in both apps. The file name must be `skin-<id>.json`. |

## How it looks: three layers

```
 ┌─────────────────────────────┐
 │  controls  (top)            │  D-pad, buttons 1-6, Coin, 1P-4P, the menu capsule, the name
 ├─────────────────────────────┤
 │  skin      (middle)         │  the plastic (painted or a picture), decor, the bezel
 ├─────────────────────────────┤
 │  video     (bottom)         │  the game, whole, as large as the skin's screen box allows
 └─────────────────────────────┘
```

The shell always looks rounded: the apps paint black outside the rim's rounded corners (radius 52), so on a phone whose screen has square corners (iPhone SE, many Android phones) the plastic never shows past the rim; the editor shows the same by drawing every phone with rounded corners. The game is always whole (never cropped, square corners) and as large as its box allows; the bezel is drawn around it, outside the box. Touch zones cover only the controls, so the picture keeps its own taps (the PIN prompt, swap offers).

## The parts of a skin

```
 portrait (402 × 778 canvas)            landscape (818 × 373 canvas)
 ┌──────────────────────────┐           ┌──────┬──────────────────────┬──────┐
 │ header (✕  room · seat)  │           │ ✕    │                      │      │
 ├──────────────────────────┤           │      │                      │  ②   │
 │                          │           │  ✚   │       screen         │ ① ③  │
 │         screen           │           │ dpad │   (menu over it,     │ ④ ⑥  │
 │     (the game, whole)    │           │      │    folds away)       │  ⑤   │
 │                          │           │ Coin │                      │1P 2P │
 ├──────────────────────────┤           └──────┴──────────────────────┴──────┘
 │     GO-LINK · ARCADE     │ label
 │  (🎤 🔊 ⏸ 💬 👥 🎮 ⚙)   │ menu (stays in portrait)
 │                          │
 │   ✚ dpad      ② ③      │ buttons: an arc of 1-6,
 │              ① ④       │ its shape depends on the game
 │                          │
 │  Coin     1P  2P  3P  4P │ coin, starts
 └──────────────────────────┘
```

## Where skins come from

- **Built in**: the files in `builtin/`. Every player has a skin: **Smoke** until they choose one (`PadSkin.defaultId`); an old saved choice of the removed "Classic" pad, or of a skin that is gone, reads as Smoke. The plain pad without a shell is only a fallback for an app that could not read any skin.
- **Installed**: the app's **Skins** folder. On iOS it is in the Files app (On My iPhone › go-link Console Player › Skins); on Android it is `Android/data/org.golink.player/files/Skins` on the phone's storage (copy files there from a computer over USB, or with `adb push`). Either a bare `<name>.json`, or a folder holding `skin.json` and its pictures. The list is read again each time Game settings opens. A file that does not parse is skipped; an installed skin cannot replace a built-in `id`. Limits: 64 KB per skin file, 8 MB per picture.
- **Pasted** (iOS and Android): Settings › **Gamepad skins** (Android: Settings › **Skins**) › **Install skin**, or the **Install skin** button under the skins in Game settings. Paste the skin's JSON (or tap **Paste**) and tap **Check**: the app parses it (a JSON mistake shows its line and column; typographic quotes from Notes are straightened), refuses a built-in `id`, runs the layout on phones and a tablet in both orientations with the same rules as the tests (warnings for parts off screen, over the picture, overlapping or smaller than a finger, and for background pictures, which a pasted file cannot bring), and draws a preview with the room's own painter in portrait and landscape with the skin's name, id and author. **Install** saves it as `Skins/skin-<id>.json` (an installed skin with the same id is replaced after asking) and offers to use it now; it shows as **Custom** in the list. Installed skins are deleted from the same screen or from their tile's menu in Game settings (built-in skins cannot be deleted). The checks are `SkinCheck` and `SkinInstaller` in GoLinkCore (`SkinInstallTests`) and `SkinInstall` in the Android core (`SkinInstallTest`); on Android a long press on a custom skin in Game settings deletes it too, after asking.
- **Where they show**: in a room with the on-screen pad, and on **Test controller** (Home), where the test card takes the game's place.
- **Choosing**: Game settings › **Skin**, per phone (`go-link.pad-skin`); nothing is sent to the device or the other players.
- **Without the pad**: with a controller in hand (or the pad turned off) there is no skin in landscape: the cinema mode shows the game as large as the screen allows with a floating menu. The gamepad button brings the skin back with a see-through pad that shows what the controller presses.

## The file

```json
{
  "format": 1,
  "id": "violet",
  "name": { "en": "Violet", "es": "Violeta", "pt": "Violeta" },
  "author": "go-link",
  "shell": { "center": "#8b2cff", "edge": "#4a0fb0", "rim": "#d9b8ff", "gloss": 0.3, "grain": 0.06 },
  "controls": "dark",
  "screen": { "bezel": "#07080c", "label": "GO-LINK · ARCADE" },
  "style": {
    "menu": { "fill": "#25075a", "fillOpacity": 0.8, "border": "#d9b8ff", "borderOpacity": 0.35,
              "button": "#160434", "icon": "#f3f0ff", "active": "#f2a33a", "activeButton": "#3a2608", "handle": "#d9b8ff" },
    "controls": { "label": "#f3f0ff" }
  },
  "background": { "portrait": "portrait.png", "landscape": "landscape.png" },
  "rings": true,
  "screws": true,
  "decor": { "portrait": [ ... ], "landscape": [ ... ] },
  "layout": { "portrait": { ... }, "landscape": { ... } }
}
```

Colors are `"#rrggbb"`. Opacities and strengths are 0 to 1. Keys the app does not know are ignored, so newer files still load in older apps; a known key with a wrong value refuses the whole file, so a broken skin never half-draws.

| Key | Required | What it is |
| --- | --- | --- |
| `format` | yes | Always `1`. |
| `id` | yes | Lowercase letters, digits and `-`, up to 40 characters; not `classic`. |
| `name` | yes | Names by language; `en` is required, `es` and `pt` are shown when the phone uses them. |
| `author` | no | Shown under the name in the list (not for `go-link`). |
| `shell` | yes | The plastic: `center` and `edge` of its radial gradient, the `rim` around the screen, and `gloss` (top highlight, default 0.3) and `grain` (default 0.06). |
| `controls` | no | The buttons' tone: `dark` (default) or `light`. |
| `style.controls` | no | The controls' own colors, each optional (the tone's otherwise): `ringTop`, `ringBottom` (the outer ring's gradient), `face`, `outline`, `mark` (D-pad arrows and dot), `label`, `lit`, `litLabel`. They color the D-pad, the round buttons and the Coin and start capsules. A held control sinks (a little smaller, its face darker with a shadow inside the rim, a shorter drop shadow; the D-pad rocks toward the held direction); `lit` adds a tint to a held control and `litLabel` colors its label, both only when the skin sets them. |
| `style.controls` (shape) | no | The controls' shapes, each optional: `shape` of the action buttons (`circle` by default, `rounded`, `hexagon`, `diamond`), `ring` (the ring's width, 0 to 0.25 of the button, default 6/64), `labels` (`numbers` 1-6, `letters` A-F, or `none`), `dome` (a convex shine on every face, 0 flat to 1), `well` (a hollow molded into the plastic around every button, capsule and the D-pad: `{"size": 0.14, "depth": 0.6, "color": "#000000"}`, size in 0 to 0.5 of the control, drawn by the app so it always fits), and `dpad` (`arm` width 0.25 to 0.6 of the D-pad, `radius` of its corners 0 to 0.5 of the arm, `marks`: `arrows`, `lines`, `dots` or `none`). A skin without them looks as before: round flat buttons with numbers and the arrow D-pad. |
| `style.menu` | no | The menu capsule: `fill`, `fillOpacity`, `border`, `borderOpacity`, the round buttons' `button` color, `icon`, `active` (an icon that is on), `activeButton`, and the folded `handle`. |
| `screen` | no | `bezel` (the frame's color, default `#07080c`) and `label`, a short text drawn where `layout.*.label` says. |
| `background` | no | A picture per orientation (PNG or JPEG), drawn like CSS `background-size: cover` in place of the painted plastic. Only for skins in their own folder: plain file names next to `skin.json`, never paths. Decor, rings, screws, gloss and the rim are still drawn over it, so a picture skin usually leaves `decor` out and sets `rings` and `screws` to false. |
| `rings` | no | A soft ring behind the D-pad and behind the action buttons (default true). |
| `screws` | no | A screw in each corner (default true). |
| `decor` | no | Up to 24 soft shapes per orientation, molded into the plastic, in 0 to 1 of the screen's width and height: `{"shape": "rect" or "grill", "x", "y", "w", "h", "radius" (points), "fill", "opacity", "stroke" (the white outline's opacity)}`. A `grill` is a speaker: rows of small holes. |
| `layout` | no | Where every part goes, per orientation (below). An orientation left out uses the automatic placement. |

## Layout: every part is a box

Each orientation is a canvas, like a page of absolutely positioned `div`s: `canvas` is the playable area of the phone the skin was drawn on (for example an iPhone 17's safe area: 402 × 778 in portrait), and every part is a box `{"x", "y", "w", "h"}` in those units, from the canvas' top left.

```json
"portrait": {
  "canvas":  { "w": 402, "h": 778 },
  "header":  { "x": 12,  "y": 0,   "w": 378, "h": 44 },
  "screen":  { "x": 0,   "y": 50,  "w": 402, "h": 302 },
  "label":   { "x": 12,  "y": 362, "w": 378, "h": 14 },
  "menu":    { "x": 12,  "y": 380, "w": 378, "h": 50, "direction": "row", "hide": false, "size": 36 },
  "dpad":    { "x": 22,  "y": 470, "w": 166, "h": 166 },
  "buttons": { "x": 214, "y": 462, "w": 176, "h": 182 },
  "coin":    { "x": 60,  "y": 712, "w": 58,  "h": 34 },
  "starts":  { "x": 128, "y": 712, "w": 266, "h": 34 },
  "pill":    { "w": 58, "h": 34 }
}
```

| Box | What goes in it |
| --- | --- |
| `screen` | The game, whole and as large as the box allows at its own aspect (never cropped, square corners), with the bezel drawn 8 points around it, outside the box. A box from edge to edge (`x` 0, `w` the canvas width) makes the picture as wide as the phone. |
| `header` | The leave button and, when wide enough, the room's name and your seat. |
| `menu` | The room's menu capsule (microphone, sound, pause, chat, players, pad, Game settings), centered in the box. `direction`: `row` or `column`. `hide`: fold it into a handle after 3 seconds without touching the picture (meant for a menu over the picture; never while VoiceOver is on). `size`: its round buttons in canvas units (28 to 60, default 36). |
| `dpad` | The D-pad, the largest square in the box. |
| `buttons` | The game's action buttons (1 to 6) in an arc sized to the box; the arc depends on how many buttons the game has. |
| `coin` | The Coin capsule, centered in the box. |
| `starts` | The 1P to 4P capsules, centered, as many per row as fit in the box's width on the canvas. |
| `pill` | The size of the Coin and start capsules (not a box: `w` 44 to 160, `h` 28 to 80; default 58 × 34). |
| `label` | Where `screen.label` is centered (optional). |

`screen`, `dpad`, `buttons`, `coin`, `starts`, `header` and `menu` are required in an orientation's layout.

**On other screens.** The canvas is stretched to the phone's playable area: box positions and regions (the screen, the header, the menu) stretch with it, while the controls keep their shape (circles stay round) and scale by the smaller of the two factors, at most 1.5 on tablets. Capsules never shrink below their size (a finger needs them), and a group that would leave the screen moves back in. In landscape the playable area reaches into the side safe areas by 45 %, beside the camera cutout. Both apps measure in points (iOS) or dp (Android), so one canvas serves both. The built-in skins are tested on several iPhone, iPad and Android sizes, both orientations, 1 to 6 buttons and 1 to 4 players: every part on screen, nothing on the picture, nothing overlapping, no control under 34 points (`SkinTests`).

Controls never shrink below a finger's size, so on a small screen they may reach the picture's box: the box then gives way on that side, the least it can, and the game fits in what is left. On a narrow screen where Coin and the start capsules would meet, the starts move aside, or Coin does when they cannot. The menu capsule shrinks below its `size` when it would not fit its box.

## Designing a skin, step by step

1. **Start from a built-in skin.** Copy `builtin/skin-smoke.json` (or open it in the editor), give it a new `id` (lowercase, digits and `-`) and names in `en`, `es` and `pt`.
2. **Colors first.** `shell.center` and `shell.edge` make the plastic's radial gradient (the edge darker), `shell.rim` the lighter border around it. Pick `controls: "dark"` for dark buttons on a light plastic's contrast, or `"light"` for white buttons; fine-tune them with `style.controls`, and the menu capsule with `style.menu` (a darker shade of the plastic reads well).
3. **Place the parts in portrait**, then **in landscape**. Each box is in canvas units (the canvas is an iPhone 17's playable area; keep it unless you draw for another shape). Rules that keep a skin comfortable:
   - the **screen** box as large as it can be: edge to edge in portrait (`x` 0, full width), the full height in landscape;
   - at least **12 units** between the screen box and any control (the bezel takes 8);
   - the **D-pad** under the left thumb and the **buttons** under the right one, roughly the same height;
   - touch targets never under **44 units** (the app keeps capsules at least 58 × 34 and buttons at least 36);
   - the **menu** under the picture in portrait (`hide: false`), over it in landscape (`hide: true`) where there is no room beside it;
   - leave the top 44 units of portrait for the header (the leave button and the room's name).
4. **Decor** is optional: soft rounded rectangles and speaker grills molded into the plastic, in 0 to 1 of the screen (they stretch with it). Keep their opacity low (0.05 to 0.15) so they never compete with the controls.
5. **A picture instead of paint** (optional): put `skin.json` and the PNG or JPEG files in one folder and name them in `background`. Draw them at 1206 × 2622 (portrait) and 2622 × 1206 (landscape) pixels, the iPhone 17 at 3×; they are drawn like CSS `cover`, so keep important art away from the edges. Leave the screen area plain: the game covers it.
6. **Check it**: validate the file with `skin.schema.json`, then copy it to the phone's Skins folder, open Game settings › Skin and choose it. Try portrait and landscape, a game with 6 buttons and one with 2, and 1 to 4 players.

## Checking a skin

- **iOS**: the debug pad lab, `-padLab -labSkin <id>` (`-labButtons N`, `-labStarts N`), draws a skin around the test card with no room; `testSkinPad` (UI tests) presses every control of each built-in skin in both orientations.
- **Android**: the debug lab `--es lab skin --es skin <id>` (`--ei buttons N`, `--ei starts N`); `npm run test:android:skins` in `e2e/` does the same presses on an emulator or phone.

## Making skins with another AI

Give it this page, [skin.schema.json](skin.schema.json) and one built-in skin as an example, then ask, for example:

> Make 5 go-link Console gamepad skins as separate JSON files following this schema: a retro beige one, a translucent teal one, a black and gold one, a Game Boy green one and a white one. Keep the built-in layout, change the colors, `style.menu`, `style.controls` and the decor. New ids, names in en, es and pt.

Check each file with any JSON Schema validator before copying it to the Skins folder; the app skips a file it cannot read.

## The editor

The editor is a page of the website: **[console.go-link.org/tools/skin-editor](https://console.go-link.org/tools/skin-editor)**, in English, Spanish and Portuguese, with nothing to install. It draws the skin exactly as the apps do (the same placement math and checks in TypeScript, `frontend/apps/web/src/skins/`, tested against the same cases as the apps) and reads the built-in skins straight from `builtin/`. It needs a computer-sized screen.

Your work stays in your browser (localStorage only, nothing is sent anywhere): **My skins** keeps several skins, each saved as it changes. **New** and **Open…** start a record, the first change to a built-in skin makes your own copy of it (built-in skins stay read-only), and the list opens, duplicates and deletes them (rename a skin through its names). To keep a skin or share it, export its JSON.

- **Start** from a built-in skin (the menu at the top left) or **Open…** a skin file with its pictures (or drop them on the page). **New** copies the skin on screen under a new id.
- **Portrait / Landscape** edit each orientation; the device menu previews on an iPhone 17, an iPhone SE, Android phones and an iPad.
- **Move** any part by dragging it (or pick it in Layers), **resize** it by its handles (hold Shift to keep its shape), nudge it with the arrows (Shift: 10). The inspector takes exact numbers.
- **Rulers** are in canvas units. Drag from the top ruler for a horizontal guide and from the left one for a vertical guide; drag a guide back onto its ruler to remove it.
- **Snap** (magnet, S) lines edges and centers up with guides, the playable area, the screen and every other part (a magenta line shows it); hold Alt to move freely. **Grid** (#) rounds to 8.
- **Plates and grills** (R, G): drag on the phone to add molded shapes to the plastic; ⌘D duplicates, ⌫ deletes.
- **Skin** (the last layer): id and names, the plastic's colors, gloss and grain, rings and screws, the screen's frame and label, the buttons' tone or own colors, the menu capsule's colors, and a background picture per orientation (drop a PNG or JPEG).
- **Preview**: 1 to 6 buttons and 1 to 4 players, a 4:3, vertical or wide game, a picture of your own in the screen (never saved), and the pressed look.
- **Buttons and D-pad** (in the Skin panel): the buttons' shape, ring, labels, convex shine and the hollow in the plastic; the D-pad's arm width, corners and marks.
- **Design, Split and Code** (1, 2, 3): the phone, the skin's JSON, or both side by side. The JSON is highlighted, a color shows its swatch, and the selected part's lines light up, so you see how each move is written. Type in it and the design follows as soon as the JSON is valid; a mistake shows its line.
- **Checks** run the apps' own tests on every change: on five screens, both orientations, every button and player count, nothing may cover the picture, overlap, leave the screen or be smaller than a finger; a warning says when controls make the picture smaller.
- **Export JSON** (⌘S) downloads `skin-<id>.json`, or `skin.json` when the skin has pictures (put it in a folder with them). Undo and redo: ⌘Z, ⇧⌘Z. Zoom: ⌘+, ⌘−, ⌘0, or ⌘ and the wheel; move around with the Hand (H) or by holding Space.

## Where the code is

| Piece | iOS | Android |
| --- | --- | --- |
| Parser, catalog, placement math | `mobile/ios/GoLinkCore/Sources/GoLinkCore/Skin.swift` | `mobile/android/core/src/main/kotlin/org/golink/player/core/Skin.kt` |
| Their tests | `GoLinkCore/Tests/GoLinkCoreTests/SkinTests.swift` | `core/src/test/.../SkinTest.kt` |
| Painter, controls, room layout | `GoLinkPlayer/UI/SkinPad.swift` | `app/src/main/.../ui/SkinPad.kt` |
| Game settings › Skin | `GoLinkPlayer/UI/GameSettingsPanel.swift` | `app/src/main/.../ui/GameSettings.kt` |

The two parsers and layouts must give the same numbers: change them together, with their tests and this page.

## Credits

The D-pad, round buttons and capsules are drawn after Kenney's **Mobile Controls** (CC0, www.kenney.nl).
