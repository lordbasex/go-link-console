// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import SwiftUI

@main
struct GoLinkPlayerApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(model)
                .preferredColorScheme(.dark)
                .tint(Tokens.accent)
                // Universal Links: https://play.go-link.org/g/<invite>.
                .onOpenURL { model.open(url: $0) }
                .onContinueUserActivity(NSUserActivityTypeBrowsingWeb) { activity in
                    if let url = activity.webpageURL { model.open(url: url) }
                }
        }
    }
}

struct RootView: View {
    @EnvironmentObject private var model: AppModel
    /** The startup intro, once per cold start (IntroView.played survives going to the background). */
    @State private var intro = !IntroView.played && !RootView.introDisabled

    var body: some View {
        ZStack {
            #if DEBUG
            if ProcessInfo.processInfo.arguments.contains("-padLab") {
                PadLabView()
            } else if ProcessInfo.processInfo.arguments.contains("-pictureLab") {
                PictureLabView()
            } else if ProcessInfo.processInfo.arguments.contains("-picture2xCheck") {
                Picture2xCheckView()
            } else if ProcessInfo.processInfo.arguments.contains("-aliasLab") {
                AliasLabView()
            } else {
                screens
            }
            #else
            screens
            #endif
            if intro {
                IntroView(playSound: model.prefs.startupSound) { intro = false }
                    .transition(.identity)
                    .zIndex(1)
            }
        }
        .onAppear { IntroView.played = true }
    }

    /** Debug runs can leave the intro out (-noIntro: UI tests; -padLab). */
    private static var introDisabled: Bool {
        #if DEBUG
        let args = ProcessInfo.processInfo.arguments
        return args.contains("-noIntro") || args.contains("-padLab") || args.contains("-aliasLab") || args.contains("-pictureLab") || args.contains("-picture2xCheck")
        #else
        return false
        #endif
    }

    private var screens: some View {
        ZStack {
            Tokens.bg.ignoresSafeArea()
            switch model.screen {
            case .home:
                HomeView()
            case .scan:
                ScanView()
            case let .join(target):
                JoinView(fixedTarget: target)
            case .room:
                if let session = model.session {
                    RoomView(session: session)
                } else {
                    Color.clear.onAppear { model.screen = .home }
                }
            case .settings:
                SettingsView()
            case .testController:
                ControllerTestView()
            }
        }
        .statusBarHidden(model.screen == .room || model.screen == .testController)
        .persistentSystemOverlays(model.screen == .room || model.screen == .testController ? .hidden : .automatic)
    }
}
