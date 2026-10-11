// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import GoLinkCore
import SwiftUI

/** The website's design tokens (frontend/packages/shared/src/tokens.css), dark theme. */
enum Tokens {
    static let bg = Color(hex: 0x0E1016)
    static let video = Color(hex: 0x05060A)
    static let surface = Color(hex: 0x161A23)
    static let surface2 = Color(hex: 0x1B2030)
    static let sunken = Color(hex: 0x131720)
    static let border = Color(hex: 0x262C3A)
    static let borderInput = Color(hex: 0x2A3040)
    static let borderStrong = Color(hex: 0x3A4256)
    static let text = Color(hex: 0xE9ECF2)
    static let text2 = Color(hex: 0xC4CAD6)
    static let muted = Color(hex: 0xA3ABBD)
    static let faint = Color(hex: 0x8C95A8)
    static let placeholder = Color(hex: 0x7F889C)
    static let dim = Color(hex: 0x5D667A)
    static let accent = Color(hex: 0xF2A33A)
    static let onAccent = Color(hex: 0x1A1206)
    static let voice = Color(hex: 0x4FC3D9)
    static let dangerBg = Color(hex: 0x2A1519)
    static let dangerBorder = Color(hex: 0x5A2A33)
    static let dangerText = Color(hex: 0xF4A9B6)
    static let accentTint = Color(hex: 0x2A1D0C)
    static let accentTintBorder = Color(hex: 0x6B4A1C)
    static let voiceTint = Color(hex: 0x10262B)
    static let rec = Color(hex: 0xE0627A)

    static func player(_ port: Int) -> Color {
        switch port {
        case 1: return Color(hex: 0xF2A33A)
        case 2: return Color(hex: 0x4FC3D9)
        case 3: return Color(hex: 0xE0627A)
        case 4: return Color(hex: 0x9D8CF0)
        default: return muted
        }
    }

    /** The website's 36 px capsule controls, 44 pt touch targets. */
    static let control: CGFloat = 44
}

extension Color {
    init(hex: UInt32, opacity: Double = 1) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255,
            opacity: opacity
        )
    }
}

/** A localized string from Localizable.strings, with printf-style values. */
func L(_ key: String, _ args: CVarArg...) -> String {
    let format = NSLocalizedString(key, comment: "")
    return args.isEmpty ? format : String(format: format, arguments: args)
}

// MARK: - Components

struct PrimaryButton: View {
    let title: String
    var icon: String?
    var enabled = true
    let action: () -> Void

    var body: some View {
        SwiftUI.Button(action: action) {
            HStack(spacing: 8) {
                if let icon { Image(systemName: icon) }
                Text(title).fontWeight(.semibold)
            }
            .frame(maxWidth: .infinity, minHeight: Tokens.control)
            .padding(.horizontal, 18)
            .foregroundStyle(enabled ? Tokens.onAccent : Tokens.faint)
            .background(Capsule().fill(enabled ? Tokens.accent : Tokens.surface2))
        }
        .buttonStyle(.plain)
        .disabled(!enabled)
    }
}

struct SecondaryButton: View {
    let title: String
    var icon: String?
    var danger = false
    var enabled = true
    let action: () -> Void

    var body: some View {
        SwiftUI.Button(action: action) {
            HStack(spacing: 8) {
                if let icon { Image(systemName: icon) }
                Text(title).fontWeight(.medium)
            }
            .frame(maxWidth: .infinity, minHeight: Tokens.control)
            .padding(.horizontal, 16)
            .foregroundStyle(danger ? Tokens.dangerText : (enabled ? Tokens.text : Tokens.faint))
            .overlay(Capsule().stroke(danger ? Tokens.dangerBorder : Tokens.borderStrong, lineWidth: 1))
            .contentShape(Capsule())
        }
        .buttonStyle(.plain)
        .disabled(!enabled)
    }
}

struct Card<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 12) { content }
            .padding(16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(RoundedRectangle(cornerRadius: 14).fill(Tokens.surface))
            .overlay(RoundedRectangle(cornerRadius: 14).stroke(Tokens.border, lineWidth: 1))
    }
}

struct Notice: View {
    let text: String
    var danger = false

    var body: some View {
        Text(text)
            .font(.subheadline)
            .foregroundStyle(danger ? Tokens.dangerText : Tokens.text2)
            .padding(12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(RoundedRectangle(cornerRadius: 10).fill(danger ? Tokens.dangerBg : Tokens.surface2))
            .overlay(RoundedRectangle(cornerRadius: 10).stroke(danger ? Tokens.dangerBorder : Tokens.border, lineWidth: 1))
    }
}

/** The go-link mark and name. */
struct Brand: View {
    var subtitle: String?

    var body: some View {
        HStack(spacing: 10) {
            Image("Logo").resizable().frame(width: 32, height: 32).accessibilityHidden(true)
            Text("go-link").font(.system(size: 20, weight: .bold)).foregroundStyle(Tokens.text)
            if let subtitle {
                Text(subtitle).font(.system(size: 13, weight: .semibold, design: .monospaced)).foregroundStyle(Tokens.accent)
            }
        }
        .accessibilityElement(children: .combine)
    }
}

/** A screen's top bar: back, title. */
struct TopBar: View {
    let title: String
    let back: () -> Void

    var body: some View {
        HStack(spacing: 4) {
            SwiftUI.Button(action: back) {
                Image(systemName: "chevron.backward").font(.system(size: 18, weight: .semibold))
                    .frame(width: Tokens.control, height: Tokens.control)
            }
            .foregroundStyle(Tokens.text)
            .accessibilityLabel(L("back"))
            .accessibilityIdentifier("back")
            Text(title).font(.title3.weight(.semibold)).foregroundStyle(Tokens.text)
            Spacer()
        }
        .padding(.horizontal, 8)
    }
}

/** A capsule text field with the website's colors. */
struct FieldStyle: ViewModifier {
    var mono = false
    var big = false

    func body(content: Content) -> some View {
        content
            .font(mono ? .system(size: big ? 24 : 18, weight: .medium, design: .monospaced) : .body)
            .foregroundStyle(Tokens.text)
            .tint(Tokens.accent)
            .padding(.horizontal, 16)
            .frame(minHeight: big ? 52 : Tokens.control)
            .background(Capsule().fill(Tokens.sunken))
            .overlay(Capsule().stroke(Tokens.borderInput, lineWidth: 1))
    }
}

extension View {
    func goLinkField(mono: Bool = false, big: Bool = false) -> some View { modifier(FieldStyle(mono: mono, big: big)) }
}

/** GoLinkCore's input bits (SwiftUI has its own Button). */
typealias PadButton = GoLinkCore.Button

/** The software keyboard's height over the window (0 when hidden). */
@MainActor
final class KeyboardHeight: ObservableObject {
    @Published private(set) var height: CGFloat = 0
    private var observers: [NSObjectProtocol] = []

    init() {
        let c = NotificationCenter.default
        observers.append(c.addObserver(forName: UIResponder.keyboardWillChangeFrameNotification, object: nil, queue: .main) { [weak self] n in
            let frame = (n.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect) ?? .zero
            let screen = UIScreen.main.bounds.height
            MainActor.assumeIsolated { self?.height = max(0, screen - frame.minY) }
        })
        observers.append(c.addObserver(forName: UIResponder.keyboardWillHideNotification, object: nil, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { self?.height = 0 }
        })
    }

    deinit { observers.forEach(NotificationCenter.default.removeObserver) }
}

/** "go-link Console Player v0.1.5 (build 105)": the bundle's MARKETING_VERSION and CURRENT_PROJECT_VERSION. */
enum AppVersion {
    static var text: String {
        let info = Bundle.main.infoDictionary ?? [:]
        let version = info["CFBundleShortVersionString"] as? String ?? "?"
        let build = info["CFBundleVersion"] as? String ?? "?"
        var s = L("app_version", version, build)
        #if DEBUG
        s += " · debug"
        #endif
        return s
    }
}

/** The app's version, small and faint; a long press copies it. */
struct AppVersionText: View {
    var body: some View {
        Text(AppVersion.text)
            .font(.caption2.monospacedDigit()).foregroundStyle(Tokens.dim)
            .frame(maxWidth: .infinity)
            .padding(.vertical, 4)
            .contextMenu {
                SwiftUI.Button(L("app_version_copy"), systemImage: "doc.on.doc") { UIPasteboard.general.string = AppVersion.text }
            }
            .accessibilityIdentifier("app-version")
    }
}
