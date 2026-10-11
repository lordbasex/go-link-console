// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import GoLinkCore
import SwiftUI

struct HomeView: View {
    @EnvironmentObject private var model: AppModel
    @State private var pasteError = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                HStack {
                    Brand(subtitle: L("app_subtitle"))
                    Spacer()
                    SwiftUI.Button { model.screen = .settings } label: {
                        Image(systemName: "gearshape").font(.system(size: 20))
                            .frame(width: Tokens.control, height: Tokens.control)
                    }
                    .foregroundStyle(Tokens.text2)
                    .accessibilityLabel(L("settings_title"))
                    .accessibilityIdentifier("home-settings")
                }
                .padding(.bottom, 12)
                if model.signal.custom {
                    ServerIndicator(url: model.signal.url) { model.setSignal(nil) }
                }
                Text(L("home_title")).font(.system(size: 30, weight: .bold)).foregroundStyle(Tokens.text)
                Text(L("home_subtitle")).font(.body).foregroundStyle(Tokens.muted)
                Card {
                    PrimaryButton(title: L("home_scan"), icon: "qrcode.viewfinder") { model.screen = .scan }
                        .accessibilityIdentifier("home-scan")
                    SecondaryButton(title: L("home_enter_code"), icon: "number") { model.screen = .join(nil) }
                        .accessibilityIdentifier("home-code")
                    SecondaryButton(title: L("home_paste"), icon: "doc.on.clipboard") {
                        let target = UIPasteboard.general.string.flatMap { Invites.typed($0) }
                        pasteError = target == nil
                        if let target { model.openInvite(target) }
                    }
                    .accessibilityIdentifier("home-paste")
                    if pasteError { Notice(text: L("home_paste_invalid"), danger: true) }
                }
                SecondaryButton(title: L("home_test_controller"), icon: "gamecontroller") { model.screen = .testController }
                    .accessibilityIdentifier("home-test-controller")
                Text(L("home_pin_note")).font(.footnote).foregroundStyle(Tokens.faint)
                Text(L("home_host_hint")).font(.footnote).foregroundStyle(Tokens.faint)
                AppVersionText().padding(.top, 8)
            }
            .frame(maxWidth: 480)
            .padding(20)
            .frame(maxWidth: .infinity)
        }
        .scrollBounceBehavior(.basedOnSize)
    }
}

/** Always visible while a custom signaling server is in use, with the way back. */
struct ServerIndicator: View {
    let url: String
    let official: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Notice(text: L("home_custom_server", url))
            SwiftUI.Button(L("settings_back_official"), action: official)
                .foregroundStyle(Tokens.accent)
                .frame(minHeight: Tokens.control)
                .accessibilityIdentifier("back-official")
        }
    }
}
