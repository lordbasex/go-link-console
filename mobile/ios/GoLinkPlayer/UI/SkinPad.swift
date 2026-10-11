// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import GoLinkCore
import SwiftUI
import UIKit

/**
 * The gamepad skins (Game settings › Skin). Every skin is a JSON file
 * (docs/skins/README.md): the built-in ones ship in the app (docs/skins/builtin,
 * "skin-<id>.json") and more can be added to the app's Skins folder in
 * the Files app (On My iPhone › go-link Console Player › Skins). The choice is
 * kept in Prefs; Smoke until the player picks one.
 */
@MainActor
final class SkinStore: ObservableObject {
    @Published private(set) var catalog: SkinCatalog
    @Published private(set) var selected: PadSkin?
    private let store: KeyValueStore

    init(store: KeyValueStore = Prefs()) {
        self.store = store
        let catalog = SkinCatalog(files: Self.files())
        self.catalog = catalog
        selected = catalog.selected(store)
    }

    var selectedId: String { selected?.id ?? "" }

    func choose(_ id: String) {
        store.set(PadSkin.prefKey, id)
        selected = catalog.selected(store)
    }

    /** Shows a skin without saving the choice (the debug lab). */
    func show(_ id: String) {
        if let skin = catalog.skin(id) { selected = skin }
    }

    /** Reads the folder again (a skin added in the Files app shows up the next time the sheet opens). */
    func reload() {
        catalog = SkinCatalog(files: Self.files())
        selected = catalog.selected(store)
    }

    /** The ids of the skins that come with the app (they cannot be replaced or deleted). */
    var builtInIds: Set<String> {
        Set((Bundle.main.urls(forResourcesWithExtension: "json", subdirectory: nil) ?? [])
            .filter { $0.lastPathComponent.hasPrefix("skin-") }
            .compactMap { (try? Data(contentsOf: $0)).flatMap { try? PadSkin.parse($0) }?.id })
    }

    /** The ids of the skins installed in the Skins folder (pasted or copied with the Files app). */
    var customIds: Set<String> {
        guard let dir = Self.installedFolder() else { return [] }
        return SkinInstaller.installedIds(in: dir).subtracting(builtInIds)
    }

    func isCustom(_ id: String) -> Bool { customIds.contains(id) }

    /** Installs a checked skin in the Skins folder (replacing one with its id); false if it could not be saved. */
    @discardableResult
    func install(_ ready: SkinCheck.Ready) -> Bool {
        guard let dir = Self.installedFolder(), (try? SkinInstaller.install(ready.data, id: ready.skin.id, into: dir)) != nil else { return false }
        reload()
        return true
    }

    /** Deletes an installed skin; the choice falls back to the default skin if it was selected. */
    func remove(_ id: String) {
        guard let dir = Self.installedFolder(), isCustom(id) else { return }
        try? SkinInstaller.remove(id: id, from: dir)
        reload()
    }

    /**
     * The app's own skins first, then the installed ones (an installed file
     * cannot replace a built-in id): a bare "<name>.json", or a folder with
     * "skin.json" and its background pictures.
     */
    private static func files() -> [SkinCatalog.File] {
        var found: [(URL, URL?)] = (Bundle.main.urls(forResourcesWithExtension: "json", subdirectory: nil) ?? [])
            .filter { $0.lastPathComponent.hasPrefix("skin-") }
            .sorted { order($0) < order($1) }
            .map { ($0, nil) }
        if let root = installedFolder(),
           let items = try? FileManager.default.contentsOfDirectory(at: root, includingPropertiesForKeys: [.isDirectoryKey]) {
            for item in items.sorted(by: { $0.lastPathComponent < $1.lastPathComponent }) {
                if (try? item.resourceValues(forKeys: [.isDirectoryKey]).isDirectory) == true {
                    found.append((item.appendingPathComponent("skin.json"), item))
                } else if item.pathExtension.lowercased() == "json" {
                    found.append((item, nil))
                }
            }
        }
        // A skin file is a few kilobytes; anything much larger is not one.
        return found.compactMap { url, folder in
            guard let data = try? Data(contentsOf: url), data.count <= 64 * 1024 else { return nil }
            return SkinCatalog.File(name: folder.map { $0.lastPathComponent + "/skin.json" } ?? url.lastPathComponent, data: data, folder: folder)
        }
    }

    private var pictures: [URL: UIImage] = [:]

    /** A skin's background picture for this orientation, read once (at most 8 MB). */
    func background(_ skin: PadSkin, landscape: Bool) -> UIImage? {
        guard let folder = skin.folder, let name = landscape ? skin.backgroundLandscape : skin.backgroundPortrait else { return nil }
        let url = folder.appendingPathComponent(name)
        if let cached = pictures[url] { return cached }
        guard let size = (try? url.resourceValues(forKeys: [.fileSizeKey]))?.fileSize, size <= 8 * 1024 * 1024,
              let image = UIImage(contentsOfFile: url.path) else { return nil }
        pictures[url] = image
        return image
    }

    /** Built-in skins in a fixed order, the rest by name. */
    private static func order(_ url: URL) -> String {
        let known = ["violet", "red", "green", "blue", "smoke", "orange"]
        let id = url.deletingPathExtension().lastPathComponent.replacingOccurrences(of: "skin-", with: "")
        if let i = known.firstIndex(of: id) { return "0\(i)" }
        return "1" + id
    }

    /** Documents/Skins, created on first use so it shows in the Files app. */
    static func installedFolder() -> URL? {
        guard let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first else { return nil }
        let dir = docs.appendingPathComponent("Skins", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir
    }
}

extension PadSkin {
    var displayName: String { name(Bundle.main.preferredLocalizations.first ?? "en") }
}

// MARK: - Shell

/**
 * The console body: the colored plastic with its molded shapes, speaker
 * grill, screws, grain, a top gloss and the rim, all from the skin file.
 */
struct SkinShell: View {
    let skin: PadSkin
    let landscape: Bool
    let layout: SkinLayout
    /** The skin's own picture for this orientation, drawn in place of the painted plastic. */
    var picture: UIImage?

    var body: some View {
        Canvas { ctx, size in
            let W = size.width, H = size.height
            let full = CGRect(origin: .zero, size: size)
            // Black outside the rim's rounded corners: on a screen with square corners
            // (iPhone SE) the plastic would show past the rim; this way the console
            // always looks rounded.
            ctx.fill(Path(full), with: .color(.black))
            ctx.clip(to: Path(roundedRect: full, cornerRadius: 52))
            if let picture {
                // Fills the screen like CSS background-size: cover.
                let k = max(W / picture.size.width, H / picture.size.height)
                let w = picture.size.width * k, h = picture.size.height * k
                ctx.draw(Image(uiImage: picture), in: CGRect(x: (W - w) / 2, y: (H - h) / 2, width: w, height: h))
            } else {
            ctx.fill(Path(full), with: .radialGradient(
                Gradient(stops: [
                    .init(color: Color(hex: skin.center), location: 0),
                    .init(color: Color(hex: skin.center), location: 0.55),
                    .init(color: Color(hex: skin.edge), location: 1),
                ]),
                center: CGPoint(x: W / 2, y: H * 0.45), startRadius: 0, endRadius: max(W, H) * 0.62
            ))
            }
            ctx.drawLayer { l in
                l.addFilter(.blur(radius: 0.8))
                for d in landscape ? skin.landscape : skin.portrait {
                    let r = CGRect(x: d.x * W, y: d.y * H, width: d.w * W, height: d.h * H)
                    let shape = Path(roundedRect: r, cornerRadius: d.radius)
                    switch d.shape {
                    case .rect:
                        l.fill(shape, with: .color(Color(hex: d.fill, opacity: d.opacity)))
                        if d.stroke > 0 { l.stroke(shape, with: .color(.white.opacity(d.stroke)), lineWidth: 1.2) }
                    case .grill:
                        l.clip(to: shape)
                        var dots = Path()
                        var y = r.minY + 4.5
                        while y < r.maxY {
                            var x = r.minX + 4.5
                            while x < r.maxX {
                                dots.addEllipse(in: CGRect(x: x - 1.7, y: y - 1.7, width: 3.4, height: 3.4))
                                x += 9
                            }
                            y += 9
                        }
                        l.fill(dots, with: .color(Color(hex: d.fill, opacity: d.opacity)))
                    }
                }
                if skin.rings {
                    for r in [layout.leftRing, layout.rightRing] {
                        let c = Path(ellipseIn: r)
                        l.fill(c, with: .color(.white.opacity(0.06)))
                        l.stroke(c, with: .color(.white.opacity(0.22)), lineWidth: 1.5)
                    }
                }
            }
            if skin.screws {
                // Inside the screen's rounded corners.
                for p in [CGPoint(x: 30, y: 30), CGPoint(x: W - 30, y: 30), CGPoint(x: 30, y: H - 30), CGPoint(x: W - 30, y: H - 30)] {
                    let c = Path(ellipseIn: CGRect(x: p.x - 6, y: p.y - 6, width: 12, height: 12))
                    ctx.fill(c, with: .color(.white.opacity(0.18)))
                    ctx.stroke(c, with: .color(.black.opacity(0.25)), lineWidth: 1)
                    var slot = Path()
                    slot.move(to: CGPoint(x: p.x - 4, y: p.y))
                    slot.addLine(to: CGPoint(x: p.x + 4, y: p.y))
                    ctx.stroke(slot, with: .color(.black.opacity(0.35)), lineWidth: 1.4)
                }
            }
            if skin.grain > 0 {
                ctx.opacity = min(1, skin.grain * 2.2)
                ctx.fill(Path(full), with: .tiledImage(Image(uiImage: Grain.image), sourceRect: CGRect(x: 0, y: 0, width: 1, height: 1), scale: 0.5))
                ctx.opacity = 1
            }
            ctx.fill(Path(full), with: .linearGradient(
                Gradient(stops: [
                    .init(color: .white.opacity(skin.gloss), location: 0),
                    .init(color: .white.opacity(skin.gloss / 6), location: 0.12),
                    .init(color: .clear, location: 0.85),
                    .init(color: .black.opacity(0.3), location: 1),
                ]),
                startPoint: .zero, endPoint: CGPoint(x: 0, y: H)
            ))
            let rim = Path(roundedRect: full.insetBy(dx: 2, dy: 2), cornerRadius: 50)
            ctx.stroke(rim, with: .color(Color(hex: skin.rim, opacity: 0.9)), lineWidth: 4)
            ctx.stroke(Path(roundedRect: full.insetBy(dx: 9, dy: 9), cornerRadius: 44), with: .color(.white.opacity(0.12)), lineWidth: 2)
        }
        .ignoresSafeArea()
        .accessibilityHidden(true)
    }
}

/** A small gray noise tile for the plastic's grain, made once. */
private enum Grain {
    static let image: UIImage = {
        let n = 96
        var bytes = [UInt8](repeating: 0, count: n * n * 4)
        var seed: UInt32 = 7
        for i in 0..<(n * n) {
            seed = seed &* 1_664_525 &+ 1_013_904_223
            let v = UInt8(truncatingIfNeeded: seed >> 24)
            bytes[i * 4] = v
            bytes[i * 4 + 1] = v
            bytes[i * 4 + 2] = v
            bytes[i * 4 + 3] = 40
        }
        let space = CGColorSpaceCreateDeviceRGB()
        let ctx = CGContext(data: &bytes, width: n, height: n, bitsPerComponent: 8, bytesPerRow: n * 4, space: space,
                            bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
        guard let cg = ctx?.makeImage() else { return UIImage() }
        return UIImage(cgImage: cg)
    }()
}

// MARK: - Controls (after Kenney's Mobile Controls, CC0)

/** The controls' colors from the skin (its tone, or its own palette). */
private struct KitColors {
    let ringTop: Color
    let ringBottom: Color
    let face: Color
    let outline: Color
    let mark: Color
    let label: Color
    /** A held control's face: the skin's tint, or the face itself a little darker (it sank). */
    let heldTop: Color
    let heldBottom: Color
    let litLabel: Color

    init(_ skin: PadSkin) {
        let p = skin.colors
        ringTop = Color(hex: p.ringTop); ringBottom = Color(hex: p.ringBottom)
        face = Color(hex: p.face); outline = Color(hex: p.outline)
        mark = Color(hex: p.mark); label = Color(hex: p.label)
        if let lit = p.lit {
            heldTop = Color(hex: lit); heldBottom = Color(hex: Self.darker(lit, 0.7))
        } else {
            // Concave: darker at the top, where the rim's shadow falls.
            heldTop = Color(hex: Self.darker(p.face, 0.74)); heldBottom = Color(hex: Self.darker(p.face, 0.9))
        }
        litLabel = Color(hex: p.litLabel)
    }

    /** The color with each channel scaled by `k`. */
    static func darker(_ rgb: UInt32, _ k: Double) -> UInt32 {
        func ch(_ shift: UInt32) -> UInt32 { UInt32(Double((rgb >> shift) & 0xFF) * k) << shift }
        return ch(16) | ch(8) | ch(0)
    }

    var ring: LinearGradient { LinearGradient(colors: [ringTop, ringBottom], startPoint: .top, endPoint: .bottom) }
    var held: LinearGradient { LinearGradient(colors: [heldTop, heldBottom], startPoint: .top, endPoint: .bottom) }
}

/** A button's outline in the skin's shape (the apps and the editor draw the same shapes). */
struct SkinButtonShape: Shape {
    let kind: PadSkin.ButtonShape

    func path(in r: CGRect) -> Path {
        switch kind {
        case .circle:
            return Path(ellipseIn: r)
        case .rounded:
            return Path(roundedRect: r, cornerRadius: min(r.width, r.height) * 0.28)
        case .hexagon:
            var p = Path()
            let c = CGPoint(x: r.midX, y: r.midY)
            let rad = min(r.width, r.height) / 2
            for i in 0..<6 {
                let a = CGFloat(-90 + 60 * Double(i)) * .pi / 180
                let pt = CGPoint(x: c.x + rad * cos(a), y: c.y + rad * sin(a))
                if i == 0 { p.move(to: pt) } else { p.addLine(to: pt) }
            }
            p.closeSubpath()
            return p
        case .diamond:
            var p = Path()
            p.move(to: CGPoint(x: r.midX, y: r.minY))
            p.addLine(to: CGPoint(x: r.maxX, y: r.midY))
            p.addLine(to: CGPoint(x: r.midX, y: r.maxY))
            p.addLine(to: CGPoint(x: r.minX, y: r.midY))
            p.closeSubpath()
            return p
        }
    }
}

/** The hollow molded into the plastic around a control: darker at the top, a light edge at the bottom. */
private struct WellView<S: Shape>: View {
    let shape: S
    let well: PadSkin.Well

    var body: some View {
        let c = Color(hex: well.color)
        shape
            .fill(LinearGradient(colors: [c.opacity(0.55 * well.depth), c.opacity(0.18 * well.depth)], startPoint: .top, endPoint: .bottom))
            .overlay(shape.stroke(LinearGradient(colors: [.black.opacity(0.45 * well.depth), .white.opacity(0.3 * well.depth)], startPoint: .top, endPoint: .bottom), lineWidth: 1.5))
    }
}

/** A convex shine over a face: light at the top, a little shade at the bottom; held, it turns over. */
private func domeGradient(_ dome: Double, held: Bool) -> LinearGradient {
    held
        ? LinearGradient(stops: [.init(color: .black.opacity(0.22 * dome), location: 0), .init(color: .clear, location: 0.6), .init(color: .white.opacity(0.14 * dome), location: 1)], startPoint: .top, endPoint: .bottom)
        : LinearGradient(stops: [.init(color: .white.opacity(0.5 * dome), location: 0), .init(color: .clear, location: 0.45), .init(color: .clear, location: 0.75), .init(color: .black.opacity(0.22 * dome), location: 1)], startPoint: .top, endPoint: .bottom)
}

/**
 * An action button in the skin's shape: a ring, a face and its label (a
 * number, a letter or nothing), optionally in a hollow of the plastic and
 * with a convex shine. Held, it sinks like a real button: a little
 * smaller, the face darker with a shadow inside its rim, the drop shadow
 * shorter.
 */
struct SkinFaceButton: View {
    @ObservedObject var state: TouchPadState
    let skin: PadSkin
    let bit: Int
    /** The button's number (1-6): its identifier; the face shows the skin's label for it. */
    let label: String
    let size: CGFloat

    var body: some View {
        let on = state.lit & bit != 0
        let k = KitColors(skin)
        let d = skin.design
        let shape = SkinButtonShape(kind: d.shape)
        let inner = size * (1 - 2 * d.ring)
        let text = d.label(Int(label) ?? 0)
        ZStack {
            if let well = d.well {
                WellView(shape: shape, well: well).frame(width: size * (1 + 2 * well.size), height: size * (1 + 2 * well.size))
            }
            // The drop shadow as its own blurred shape: a .shadow on the
            // scaled button left a square smudge behind it while held.
            shape.fill(.black.opacity(on ? 0.3 : 0.4))
                .frame(width: size, height: size)
                .blur(radius: on ? 1.5 : 5)
                .offset(y: on ? 1 : 5)
            ZStack {
                shape.fill(k.ring).frame(width: size, height: size)
                shape.fill(on ? AnyShapeStyle(k.held) : AnyShapeStyle(k.face)).frame(width: inner, height: inner)
                if d.dome > 0 { shape.fill(domeGradient(d.dome, held: on)).frame(width: inner, height: inner) }
                if on {
                    shape.stroke(.black.opacity(0.4), lineWidth: size * 0.06)
                        .blur(radius: size * 0.03)
                        .clipShape(shape)
                        .frame(width: inner, height: inner)
                }
                shape.stroke(k.outline, lineWidth: 1).frame(width: inner, height: inner)
                if !text.isEmpty {
                    Text(text)
                        .font(.system(size: size * 0.36, weight: .bold, design: .rounded))
                        .foregroundStyle(on ? k.litLabel.opacity(0.8) : k.label)
                        .offset(y: on ? 1 : 0)
                }
            }
            .scaleEffect(on ? 0.93 : 1)
        }
        .frame(width: size, height: size)
        .animation(.easeOut(duration: 0.05), value: on)
        .background(PadAnchor(state: state, bit: bit))
        .accessibilityElement()
        .accessibilityLabel(text.isEmpty ? label : text)
        .accessibilityIdentifier("pad-button-\(label)")
    }
}

/** Coin and the start buttons: a wide rounded button with its name; the skin's hollow and shine apply too. */
struct SkinPillButton: View {
    @ObservedObject var state: TouchPadState
    let skin: PadSkin
    let bit: Int
    let label: String
    var mine = false
    let tag: String
    var size = SkinLayout.pill

    var body: some View {
        let on = state.lit & bit != 0
        let k = KitColors(skin)
        let d = skin.design
        let s = size
        let inset = s.height * 5 / 64
        ZStack {
            if let well = d.well {
                WellView(shape: Capsule(), well: well).frame(width: s.width + 2 * s.height * well.size, height: s.height * (1 + 2 * well.size))
            }
            Capsule().fill(.black.opacity(on ? 0.3 : 0.4))
                .blur(radius: on ? 1.5 : 4)
                .offset(y: on ? 1 : 3)
            ZStack {
                Capsule().fill(k.ring)
                Capsule().inset(by: inset).fill(on ? AnyShapeStyle(k.held) : AnyShapeStyle(k.face))
                if d.dome > 0 { Capsule().inset(by: inset).fill(domeGradient(d.dome, held: on)) }
                if on {
                    Capsule().inset(by: inset).stroke(.black.opacity(0.4), lineWidth: s.height * 0.08)
                        .blur(radius: s.height * 0.04)
                        .clipShape(Capsule().inset(by: inset))
                }
                // Your own start keeps the app's accent ring.
                Capsule().inset(by: inset).stroke(mine ? Tokens.accent : k.outline, lineWidth: mine ? 1.5 : 1)
                Text(label)
                    .font(.system(size: s.height * 0.36, weight: .bold, design: .rounded))
                    .tracking(0.6)
                    .lineLimit(1).minimumScaleFactor(0.7)
                    .foregroundStyle(on ? k.litLabel.opacity(0.8) : k.label)
                    .offset(y: on ? 1 : 0)
                    .padding(.horizontal, 6)
            }
            .frame(width: s.width, height: s.height)
            .scaleEffect(on ? 0.95 : 1)
        }
        .frame(width: s.width, height: s.height)
        .animation(.easeOut(duration: 0.05), value: on)
        .background(PadAnchor(state: state, bit: bit))
        .accessibilityElement()
        .accessibilityLabel(label)
        .accessibilityIdentifier(tag)
    }
}

/**
 * The D-pad: a dark well (or the skin's hollow) and the cross, with the
 * skin's arm width, corners and marks (arrows, lines, dots or none) and
 * its convex shine. Held, the cross rocks toward the direction like a real
 * one (a small 3D tilt, diagonals too) and the held arm sinks darker.
 */
struct SkinDPad: View {
    @ObservedObject var state: TouchPadState
    let skin: PadSkin
    let size: CGFloat

    /** Which way the cross rocks: x right (+) or left, y down (+) or up. */
    private var tilt: CGPoint {
        let h = state.lit
        let x = (h & PadButton.right != 0 ? 1 : 0) - (h & PadButton.left != 0 ? 1 : 0)
        let y = (h & PadButton.down != 0 ? 1 : 0) - (h & PadButton.up != 0 ? 1 : 0)
        return CGPoint(x: x, y: y)
    }

    /** Kenney's 128-unit D-pad, scaled, with the skin's arm width, corners, marks and shine. */
    private static func drawCross(_ ctx: inout GraphicsContext, _ sz: CGSize, held: Int, k: KitColors, d: PadSkin.ControlDesign) {
        // Kenney's 128-unit D-pad, scaled, with the skin's arm width and corners.
        let u = sz.width / 128
        let arm = CGFloat(d.dpadArm) * 128
        let lo: CGFloat = 64 - arm / 2
        let hi: CGFloat = 64 + arm / 2
        let e: CGFloat = 6
        let corner = CGFloat(d.dpadRadius) * arm * u
        func r(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat) -> CGRect { CGRect(x: x * u, y: y * u, width: w * u, height: h * u) }
        var body = Path()
        body.addRoundedRect(in: r(lo, 0, arm, 128), cornerSize: CGSize(width: corner, height: corner))
        body.addRoundedRect(in: r(0, lo, 128, arm), cornerSize: CGSize(width: corner, height: corner))
        ctx.fill(body, with: .linearGradient(Gradient(colors: [k.ringTop, k.ringBottom]), startPoint: .zero, endPoint: CGPoint(x: 0, y: sz.height)))
        var face = Path()
        let a = lo + e, b = hi - e, f: CGFloat = 128 - e
        let outline: [CGPoint] = [
            CGPoint(x: e, y: a), CGPoint(x: a, y: a), CGPoint(x: a, y: e), CGPoint(x: b, y: e), CGPoint(x: b, y: a), CGPoint(x: f, y: a),
            CGPoint(x: f, y: b), CGPoint(x: b, y: b), CGPoint(x: b, y: f), CGPoint(x: a, y: f), CGPoint(x: a, y: b), CGPoint(x: e, y: b),
        ]
        face.addLines(outline.map { CGPoint(x: $0.x * u, y: $0.y * u) })
        face.closeSubpath()
        ctx.fill(face, with: .color(k.face))
        let side = arm - 2 * e
        let arms: [(Int, CGRect)] = [
            (PadButton.up, r(lo + e, e, side, lo)), (PadButton.down, r(lo + e, hi - e, side, 128 - hi)),
            (PadButton.left, r(e, lo + e, lo, side)), (PadButton.right, r(hi - e, lo + e, 128 - hi, side)),
        ]
        for (bit, rect) in arms where held & bit != 0 {
            ctx.fill(Path(rect), with: .linearGradient(Gradient(colors: [k.heldTop, k.heldBottom]), startPoint: CGPoint(x: rect.midX, y: rect.minY), endPoint: CGPoint(x: rect.midX, y: rect.maxY)))
        }
        if d.dome > 0 { ctx.fill(face, with: .linearGradient(Gradient(stops: [.init(color: .white.opacity(0.5 * d.dome), location: 0), .init(color: .clear, location: 0.45), .init(color: .clear, location: 0.75), .init(color: .black.opacity(0.22 * d.dome), location: 1)]), startPoint: .zero, endPoint: CGPoint(x: 0, y: sz.height))) }
        ctx.stroke(face, with: .color(k.outline), lineWidth: 1)
        let hw: CGFloat = min(5, arm / 2 - e - 2)
        func poly(_ pts: [(CGFloat, CGFloat)]) {
            var p = Path()
            p.addLines(pts.map { CGPoint(x: $0.0 * u, y: $0.1 * u) })
            p.closeSubpath()
            ctx.fill(p, with: .color(k.mark))
        }
        switch d.dpadMarks {
        case .arrows:
            ctx.fill(Path(ellipseIn: r(61, 61, 6, 6)), with: .color(k.mark))
            poly([(64, 19), (64 + hw, 27), (64 - hw, 27)])
            poly([(64, 109), (64 + hw, 101), (64 - hw, 101)])
            poly([(19, 64), (27, 64 - hw), (27, 64 + hw)])
            poly([(109, 64), (101, 64 - hw), (101, 64 + hw)])
        case .lines:
            for (a, b) in [((64.0, 16.0), (64.0, 34.0)), ((64.0, 94.0), (64.0, 112.0)), ((16.0, 64.0), (34.0, 64.0)), ((94.0, 64.0), (112.0, 64.0))] {
                var p = Path()
                p.move(to: CGPoint(x: a.0 * u, y: a.1 * u))
                p.addLine(to: CGPoint(x: b.0 * u, y: b.1 * u))
                ctx.stroke(p, with: .color(k.mark), style: StrokeStyle(lineWidth: 4 * u, lineCap: .round))
            }
        case .dots:
            for (x, y) in [(64.0, 64.0), (64.0, 24.0), (64.0, 104.0), (24.0, 64.0), (104.0, 64.0)] {
                ctx.fill(Path(ellipseIn: r(x - 3.5, y - 3.5, 7, 7)), with: .color(k.mark))
            }
        case .none:
            break
        }
    }

    var body: some View {
        let held = state.lit
        let k = KitColors(skin)
        let d = skin.design
        ZStack {
            if let well = d.well {
                WellView(shape: Circle(), well: well)
            } else {
                Circle().fill(.black.opacity(0.2))
                    .overlay(Circle().stroke(.white.opacity(0.12), lineWidth: 2))
                    .shadow(color: .black.opacity(0.35), radius: 8, y: 4)
            }
            Canvas { ctx, sz in Self.drawCross(&ctx, sz, held: held, k: k, d: d) }
            .frame(width: size * 0.86, height: size * 0.86)
            .rotation3DEffect(.degrees(tilt.x == 0 && tilt.y == 0 ? 0 : 9), axis: (x: tilt.y, y: tilt.x, z: 0), perspective: 0.6)
            .shadow(color: .black.opacity(0.5), radius: tilt == .zero ? 6 : 4, y: tilt == .zero ? 5 : 3)
            .animation(.easeOut(duration: 0.05), value: tilt)
        }
        .frame(width: size, height: size)
        .background(PadAnchor(state: state, isDpad: true))
        .accessibilityElement()
        .accessibilityIdentifier("pad-dpad")
    }
}

// MARK: - Layout

/**
 * The room with a skin: the shell fills the screen, the picture sits in
 * its bezel (whole, never cropped), and every part goes where the skin
 * file puts it for this orientation (SkinLayout): the D-pad, the action
 * buttons, Coin and the starts, the room's name and the menu capsule. A
 * menu the skin marks "hide" folds into a handle after 3 s without
 * touching the picture (never while VoiceOver is on); a tap on the
 * picture or the handle brings it back, the pad's own buttons never do.
 */
struct SkinConsoleLayout: View {
    let skin: PadSkin
    var picture: UIImage?
    let landscape: Bool
    /** The safe area's size and its insets: the shell covers the whole screen, the controls stay reachable. */
    let size: CGSize
    let insets: EdgeInsets
    let pad: TouchPadState
    let controls: GameControls
    let starts: Int
    let myPorts: [Int]
    let aspect: CGFloat
    /** Keeps the menu shown (a sheet or the drawer is open). */
    var keepDock = false
    /** Each start pill's button and label: the panel's 1P-4P in a room, the player's own Start in the controller test. */
    var startBit: (Int) -> Int = { startOf($0) }
    var startLabel: (Int) -> String = { L("room_start_player", $0) }
    let header: (_ compact: Bool) -> AnyView
    let screen: AnyView
    let dock: (_ vertical: Bool) -> AnyView
    @State private var dockShown = true
    @State private var hideTask: Task<Void, Never>?
    /** The menu capsule's own size before scaling, measured, so it can shrink to fit its box. */
    @State private var menuSize: CGSize = .zero

    private static let idle: UInt64 = 3_000_000_000

    var body: some View {
        let full = CGSize(width: size.width + insets.leading + insets.trailing, height: size.height + insets.top + insets.bottom)
        let bits = TouchPadLogic.actionButtons(controls.buttons)
        let l = SkinLayout.compute(
            size: full, insets: (insets.top, insets.leading, insets.bottom, insets.trailing),
            landscape: landscape, aspect: aspect, buttons: bits.count, starts: starts,
            placement: landscape ? skin.landscapePlacement : skin.portraitPlacement
        )
        let pill = l.coin.size
        ZStack(alignment: .topLeading) {
            SkinShell(skin: skin, landscape: landscape, layout: l, picture: picture)
            // The controls, drawn as the skin places them.
            DisplayOnlyFade(state: pad) {
            ZStack(alignment: .topLeading) {
                Color.clear
                SkinDPad(state: pad, skin: skin, size: l.dpad.width).position(x: l.dpad.midX, y: l.dpad.midY)
                ForEach(Array(zip(bits, l.faces).enumerated()), id: \.offset) { i, pair in
                    SkinFaceButton(state: pad, skin: skin, bit: pair.0, label: "\(i + 1)", size: pair.1.width)
                        .position(x: pair.1.midX, y: pair.1.midY)
                }
                SkinPillButton(state: pad, skin: skin, bit: PadButton.coin, label: L("room_coin"), tag: "pad-coin", size: pill)
                    .position(x: l.coin.midX, y: l.coin.midY)
                ForEach(Array(l.starts.enumerated()), id: \.offset) { i, r in
                    let port = i + 1
                    SkinPillButton(state: pad, skin: skin, bit: startBit(port), label: startLabel(port), mine: myPorts.contains(port), tag: "pad-start-\(port)", size: pill)
                        .position(x: r.midX, y: r.midY)
                }
            }
            .frame(width: full.width, height: full.height, alignment: .topLeading)
            }
            .allowsHitTesting(false)
            // Fingers land on touch zones over the controls only, never over the picture
            // (its overlays and the menu keep their taps). A finger that started in a zone
            // keeps sliding across every control.
            ForEach(Array(touchZones(l).enumerated()), id: \.offset) { _, zone in
                PadSurface(state: pad) { Color.clear.contentShape(Rectangle()) }
                    .frame(width: zone.width, height: zone.height)
                    .position(x: zone.midX, y: zone.midY)
            }
            bezel(l)
            screen
                .frame(width: l.screen.width, height: l.screen.height)
                // Square corners: the whole game shows, no corner is cut.
                .clipped()
                .simultaneousGesture(TapGesture().onEnded { if l.menuHides { wake() } })
                .position(x: l.screen.midX, y: l.screen.midY)
            if let label = l.label, !skin.label.isEmpty {
                Text(skin.label)
                    .font(.system(size: 10, weight: .bold, design: .monospaced)).tracking(3)
                    .foregroundStyle(.white.opacity(0.45))
                    .lineLimit(1)
                    .position(x: label.x, y: label.y)
                    .accessibilityHidden(true)
            }
            header(landscape)
                .frame(width: l.header.width, height: l.header.height, alignment: .leading)
                .position(x: l.header.midX, y: l.header.midY)
            menu(l)
                .position(x: l.menu.midX, y: l.menu.midY)
        }
        .frame(width: full.width, height: full.height, alignment: .topLeading)
        .ignoresSafeArea()
        .onAppear { wake() }
        .onDisappear { hideTask?.cancel() }
        .onChange(of: keepDock) { _, _ in wake() }
        .onChange(of: landscape) { _, _ in wake() }
    }

    /** The D-pad, the action buttons as one block, Coin and each start, with a margin for thumbs. */
    private func touchZones(_ l: SkinLayout) -> [CGRect] {
        let faces = l.faces.dropFirst().reduce(l.faces.first ?? .zero) { $0.union($1) }
        return ([l.dpad, faces, l.coin] + l.starts).map { $0.insetBy(dx: -8, dy: -8) }
    }

    private func bezel(_ l: SkinLayout) -> some View {
        let r = l.screen.insetBy(dx: -8, dy: -8)
        return RoundedRectangle(cornerRadius: 18)
            .fill(Color(hex: skin.bezel))
            .overlay(RoundedRectangle(cornerRadius: 18).stroke(.white.opacity(0.07), lineWidth: 1))
            .shadow(color: .black.opacity(0.45), radius: 13, y: 10)
            .frame(width: r.width, height: r.height)
            .position(x: r.midX, y: r.midY)
            .accessibilityHidden(true)
    }

    /** The menu capsule, sized and colored by the skin, or its handle while folded. */
    @ViewBuilder private func menu(_ l: SkinLayout) -> some View {
        let m = skin.menuStyle
        // The skin's button size, smaller when the capsule would not fit its box (narrow phones).
        let fit = menuSize.width > 0 ? min(l.menu.width / menuSize.width, l.menu.height / menuSize.height) : 1
        let k = min(l.menuButton / Tokens.control, fit)
        if !l.menuHides || dockShown || keepDock || UIAccessibility.isVoiceOverRunning {
            dock(l.menuVertical)
                .environment(\.dockStyle, m)
                .fixedSize()
                .padding(6)
                .background(Capsule().fill(Color(hex: m.fill, opacity: m.fillOpacity)))
                .overlay(Capsule().stroke(Color(hex: m.border, opacity: m.borderOpacity), lineWidth: 1))
                .background(GeometryReader { g in
                    Color.clear.onAppear { menuSize = g.size }.onChange(of: g.size) { _, s in menuSize = s }
                })
                .scaleEffect(k)
                .frame(width: l.menu.width, height: l.menu.height)
                .transition(.opacity)
        } else {
            SwiftUI.Button(action: wake) {
                Capsule().fill(Color(hex: m.handle, opacity: 0.4))
                    .frame(width: l.menuVertical ? 6 : 46, height: l.menuVertical ? 46 : 6)
                    .frame(width: l.menuVertical ? Tokens.control : 88, height: l.menuVertical ? 88 : Tokens.control)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .frame(width: l.menu.width, height: l.menu.height, alignment: l.menuVertical ? .center : .top)
            .accessibilityLabel(L("room_show_menu"))
            .accessibilityIdentifier("dock-handle")
            .transition(.opacity)
        }
    }

    /** Shows the menu and, when the skin folds it, hides it again after 3 s without touching the picture. */
    private func wake() {
        withAnimation(.easeOut(duration: 0.2)) { dockShown = true }
        hideTask?.cancel()
        hideTask = Task { @MainActor in
            try? await Task.sleep(nanoseconds: Self.idle)
            guard !Task.isCancelled, !keepDock else { return }
            withAnimation(.easeOut(duration: 0.3)) { dockShown = false }
        }
    }
}

/** See-through while the pad only shows a real controller's presses. */
private struct DisplayOnlyFade<Content: View>: View {
    @ObservedObject var state: TouchPadState
    @ViewBuilder let content: Content

    var body: some View { content.opacity(state.displayOnly ? 0.55 : 1) }
}
