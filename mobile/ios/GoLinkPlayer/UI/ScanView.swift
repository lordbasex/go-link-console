// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import AVFoundation
import GoLinkCore
import SwiftUI

/**
 * The QR code scanner. It explains the camera first; the system asks
 * next. Only a go-link invitation is accepted (Invites.scanned, which
 * also reads a group invitation's key after #k=):
 * another site, a signaling server or a PIN in a QR code is refused.
 */
struct ScanView: View {
    @EnvironmentObject private var model: AppModel
    @State private var status = AVCaptureDevice.authorizationStatus(for: .video)
    @State private var invalid = false
    private let hasCamera = AVCaptureDevice.default(for: .video) != nil

    var body: some View {
        VStack(spacing: 0) {
            TopBar(title: L("scan_title")) { model.screen = .home }
            content
        }
    }

    @ViewBuilder
    private var content: some View {
        if status == .notDetermined {
            PermissionExplainer(
                icon: "camera",
                title: L("perm_camera_title"),
                text: L("perm_camera_text"),
                allow: L("perm_allow"),
                notNow: L("perm_not_now"),
                note: nil,
                onAllow: {
                    AVCaptureDevice.requestAccess(for: .video) { _ in
                        DispatchQueue.main.async { status = AVCaptureDevice.authorizationStatus(for: .video) }
                    }
                },
                onNotNow: { model.screen = .join(nil) }
            )
        } else if !hasCamera {
            message(L("scan_no_camera"))
        } else if status == .authorized {
            ZStack(alignment: .bottom) {
                QrScanner { text in
                    if let target = Invites.scanned(text) {
                        model.openInvite(target)
                    } else {
                        invalid = true
                    }
                }
                .ignoresSafeArea(edges: .bottom)
                RoundedRectangle(cornerRadius: 24).stroke(Tokens.accent, lineWidth: 3)
                    .frame(width: 240, height: 240)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .allowsHitTesting(false)
                VStack(spacing: 10) {
                    Text(L(invalid ? "scan_invalid" : "scan_hint"))
                        .font(.subheadline)
                        .foregroundStyle(invalid ? Tokens.dangerText : Tokens.text)
                        .multilineTextAlignment(.center)
                        .accessibilityIdentifier("scan-status")
                    SecondaryButton(title: L("scan_type_instead")) { model.screen = .join(nil) }
                }
                .padding(16)
                .background(RoundedRectangle(cornerRadius: 16).fill(Tokens.surface.opacity(0.92)))
                .padding(16)
            }
        } else {
            PermissionExplainer(
                icon: "camera",
                title: L("perm_camera_title"),
                text: L("perm_camera_text"),
                allow: L("perm_open_settings"),
                notNow: L("scan_type_instead"),
                note: L("perm_camera_denied"),
                onAllow: { if let url = URL(string: UIApplication.openSettingsURLString) { UIApplication.shared.open(url) } },
                onNotNow: { model.screen = .join(nil) }
            )
        }
    }

    private func message(_ text: String) -> some View {
        VStack(spacing: 16) {
            Image(systemName: "camera").font(.system(size: 40)).foregroundStyle(Tokens.faint)
            Text(text).foregroundStyle(Tokens.text2).multilineTextAlignment(.center)
                .accessibilityIdentifier("scan-status")
            PrimaryButton(title: L("scan_type_instead")) { model.screen = .join(nil) }
                .frame(maxWidth: 320)
        }
        .padding(24)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/** Why a permission is needed, before the system asks. */
struct PermissionExplainer: View {
    let icon: String
    let title: String
    let text: String
    let allow: String
    let notNow: String
    let note: String?
    let onAllow: () -> Void
    let onNotNow: () -> Void

    var body: some View {
        ScrollView {
            VStack(spacing: 16) {
                Image(systemName: icon).font(.system(size: 34, weight: .medium)).foregroundStyle(Tokens.accent)
                    .frame(width: 72, height: 72)
                    .background(Circle().fill(Tokens.accentTint))
                    .overlay(Circle().stroke(Tokens.accentTintBorder, lineWidth: 1))
                Text(title).font(.title3.weight(.semibold)).foregroundStyle(Tokens.text).multilineTextAlignment(.center)
                Text(text).foregroundStyle(Tokens.muted).multilineTextAlignment(.center)
                if let note { Notice(text: note, danger: true) }
                PrimaryButton(title: allow, action: onAllow).accessibilityIdentifier("perm-allow")
                SecondaryButton(title: notNow, action: onNotNow).accessibilityIdentifier("perm-not-now")
            }
            .frame(maxWidth: 420)
            .padding(24)
            .frame(maxWidth: .infinity)
        }
    }
}

/** The camera preview with a QR code reader (AVFoundation metadata). */
struct QrScanner: UIViewRepresentable {
    let onCode: (String) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(onCode: onCode) }

    func makeUIView(context: Context) -> PreviewView {
        let view = PreviewView()
        context.coordinator.start(in: view)
        return view
    }

    func updateUIView(_ view: PreviewView, context: Context) {}

    static func dismantleUIView(_ view: PreviewView, coordinator: Coordinator) { coordinator.stop() }

    final class PreviewView: UIView {
        override class var layerClass: AnyClass { AVCaptureVideoPreviewLayer.self }
        var preview: AVCaptureVideoPreviewLayer { layer as! AVCaptureVideoPreviewLayer }
    }

    final class Coordinator: NSObject, AVCaptureMetadataOutputObjectsDelegate {
        private let session = AVCaptureSession()
        private let onCode: (String) -> Void
        private var last = ""

        init(onCode: @escaping (String) -> Void) { self.onCode = onCode }

        func start(in view: PreviewView) {
            guard let camera = AVCaptureDevice.default(for: .video),
                  let input = try? AVCaptureDeviceInput(device: camera),
                  session.canAddInput(input)
            else { return }
            session.addInput(input)
            let output = AVCaptureMetadataOutput()
            guard session.canAddOutput(output) else { return }
            session.addOutput(output)
            output.setMetadataObjectsDelegate(self, queue: .main)
            output.metadataObjectTypes = [.qr]
            view.preview.session = session
            view.preview.videoGravity = .resizeAspectFill
            DispatchQueue.global(qos: .userInitiated).async { [session] in session.startRunning() }
        }

        func stop() {
            DispatchQueue.global(qos: .userInitiated).async { [session] in session.stopRunning() }
        }

        func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput objects: [AVMetadataObject], from connection: AVCaptureConnection) {
            guard let code = objects.compactMap({ ($0 as? AVMetadataMachineReadableCodeObject)?.stringValue }).first, code != last else { return }
            last = code
            onCode(code)
        }
    }
}
