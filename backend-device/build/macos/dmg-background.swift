// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Draws the background of the macOS installer disk image in the go-link
// style (the dark page color, an amber glow, an amber arrow) and writes a
// multi-resolution TIFF (1x and 2x, crisp on Retina Finder windows).
//
//   swift build/macos/dmg-background.swift OUT.tiff
//
// Layout (points, must match make-dmg.sh): a 660x400 window, go-link-console.app
// at (165, 190), Applications at (495, 190), the arrow between them.
import AppKit

let args = CommandLine.arguments
guard args.count == 2 else {
    FileHandle.standardError.write("usage: dmg-background.swift OUT.tiff\n".data(using: .utf8)!)
    exit(2)
}
let output = URL(fileURLWithPath: args[1])
let work = output.deletingLastPathComponent()
let width = 660.0, height = 400.0

func color(_ hex: Int, _ alpha: CGFloat = 1) -> CGColor {
    CGColor(red: CGFloat((hex >> 16) & 0xff) / 255, green: CGFloat((hex >> 8) & 0xff) / 255,
            blue: CGFloat(hex & 0xff) / 255, alpha: alpha)
}

func render(scale: CGFloat) -> CGImage {
    let space = CGColorSpace(name: CGColorSpace.sRGB)!
    let ctx = CGContext(data: nil, width: Int(width * scale), height: Int(height * scale), bitsPerComponent: 8,
                        bytesPerRow: 0, space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)!
    ctx.scaleBy(x: scale, y: scale)

    // The site's dark background: surface at the top left, page color below.
    let base = CGGradient(colorsSpace: space, colors: [color(0x1b2030), color(0x0e1016)] as CFArray, locations: [0, 1])!
    ctx.drawLinearGradient(base, start: CGPoint(x: 0, y: height), end: CGPoint(x: width, y: 0), options: [])
    // An amber glow, like the splash and the logo's shadow.
    let glow = CGGradient(colorsSpace: space, colors: [color(0xf2a33a, 0.22), color(0xf2a33a, 0)] as CFArray, locations: [0, 1])!
    ctx.drawRadialGradient(glow, startCenter: CGPoint(x: width * 0.3, y: height * 0.8), startRadius: 0,
                           endCenter: CGPoint(x: width * 0.3, y: height * 0.8), endRadius: 360, options: [])

    // A faint pixel grid, a nod to arcade screens.
    ctx.setFillColor(color(0xffffff, 0.035))
    var gx = 12.0
    while gx < width {
        var gy = 12.0
        while gy < height {
            ctx.fill(CGRect(x: gx, y: gy, width: 1.5, height: 1.5))
            gy += 24
        }
        gx += 24
    }

    // Soft light behind the two icons.
    for cx in [165.0, 495.0] {
        let spot = CGGradient(colorsSpace: space, colors: [color(0xffffff, 0.08), color(0xffffff, 0)] as CFArray, locations: [0, 1])!
        ctx.drawRadialGradient(spot, startCenter: CGPoint(x: cx, y: height - 190), startRadius: 0,
                               endCenter: CGPoint(x: cx, y: height - 190), endRadius: 110, options: [])
    }

    // The arrow from the app to Applications, in the accent color.
    let y = height - 190
    let arrow = CGMutablePath()
    arrow.move(to: CGPoint(x: 258, y: y))
    arrow.addLine(to: CGPoint(x: 392, y: y))
    arrow.move(to: CGPoint(x: 370, y: y + 18))
    arrow.addLine(to: CGPoint(x: 396, y: y))
    arrow.addLine(to: CGPoint(x: 370, y: y - 18))
    ctx.addPath(arrow)
    ctx.setLineWidth(6)
    ctx.setLineCap(.round)
    ctx.setLineJoin(.round)
    ctx.setStrokeColor(color(0xf2a33a))
    ctx.strokePath()

    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(cgContext: ctx, flipped: false)
    func draw(_ text: String, size: CGFloat, weight: NSFont.Weight, color: NSColor, y: CGFloat) {
        let style = NSMutableParagraphStyle()
        style.alignment = .center
        let attrs: [NSAttributedString.Key: Any] = [.font: NSFont.systemFont(ofSize: size, weight: weight),
                                                    .foregroundColor: color, .paragraphStyle: style]
        NSAttributedString(string: text, attributes: attrs).draw(in: CGRect(x: 0, y: y, width: width, height: size * 1.4))
    }
    draw("Drag go-link to your Applications folder", size: 15, weight: .semibold,
         color: NSColor(white: 1, alpha: 0.92), y: 62)
    draw("Your arcade, online. Play together from anywhere.", size: 12, weight: .regular,
         color: NSColor(red: 0.95, green: 0.64, blue: 0.23, alpha: 0.85), y: 40)
    NSGraphicsContext.restoreGraphicsState()
    return ctx.makeImage()!
}

try? FileManager.default.createDirectory(at: work, withIntermediateDirectories: true)
var pngs: [URL] = []
for scale in [1.0, 2.0] as [CGFloat] {
    let url = work.appendingPathComponent(scale == 1 ? "background.png" : "background@2x.png")
    let rep = NSBitmapImageRep(cgImage: render(scale: scale))
    rep.size = NSSize(width: width, height: height) // points, so the DPI tags the scale
    try! rep.representation(using: .png, properties: [:])!.write(to: url)
    pngs.append(url)
}
// tiffutil folds both scales into one TIFF; the Finder picks the right one.
let task = Process()
task.executableURL = URL(fileURLWithPath: "/usr/bin/tiffutil")
task.arguments = ["-cathidpicheck", pngs[0].path, pngs[1].path, "-out", output.path]
try! task.run()
task.waitUntilExit()
exit(task.terminationStatus)
