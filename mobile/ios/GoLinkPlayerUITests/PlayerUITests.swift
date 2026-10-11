// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest

/**
 * Drives go-link Console Player in the simulator. testScreens saves screenshots of
 * the main screens (GL_SHOTS: folder, GL_LANG: en/es/pt). testLiveJoin
 * joins a real room when GL_INVITE (https://play.go-link.org/g/<invite>) and
 * GL_PIN are given, and is skipped otherwise. testPadAcrossRotations
 * rotates the debug pad lab and presses the on-screen pad. Pass them to xcodebuild as
 * TEST_RUNNER_GL_* environment variables.
 */
final class PlayerUITests: XCTestCase {
    private var env: [String: String] { ProcessInfo.processInfo.environment }
    private var lang: String { env["GL_LANG"] ?? "en" }

    override func setUp() {
        continueAfterFailure = false
    }

    private func launch(_ extra: [String] = []) -> XCUIApplication {
        let app = XCUIApplication()
        // -noIntro: no startup intro over the screens under test.
        app.launchArguments = ["-noIntro", "-AppleLanguages", "(\(lang))", "-AppleLocale", lang == "en" ? "en_US" : (lang == "es" ? "es_ES" : "pt_BR")] + extra
        app.launch()
        return app
    }

    private func shot(_ name: String) {
        let png = XCUIScreen.main.screenshot().pngRepresentation
        let attachment = XCTAttachment(data: png, uniformTypeIdentifier: "public.png")
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        if let dir = env["GL_SHOTS"] {
            try? FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
            try? png.write(to: URL(fileURLWithPath: dir).appendingPathComponent("\(lang)-\(name).png"))
        }
    }

    func testScreens() {
        let app = launch(["-resetState"])
        XCTAssertTrue(app.buttons["home-scan"].waitForExistence(timeout: 10))
        shot("1-home")

        app.buttons["home-scan"].tap()
        XCTAssertTrue(app.buttons["perm-allow"].waitForExistence(timeout: 5) || app.staticTexts["scan-status"].exists)
        shot("2-scanner-permission")
        app.buttons["back"].tap()

        app.buttons["home-code"].tap()
        let code = app.textFields["join-code"]
        XCTAssertTrue(code.waitForExistence(timeout: 5))
        code.tap()
        code.typeText("915355636")
        // The digits show grouped while they are typed.
        XCTAssertEqual(code.value as? String, "915 355 636")
        shot("3-code-entry")
        let pin = app.textFields["join-pin"]
        pin.tap()
        pin.typeText("482913")
        app.buttons["join-button"].tap() // terms not accepted yet: asks for them
        shot("4-pin")
        app.buttons["back"].tap()

        app.buttons["home-settings"].tap()
        XCTAssertTrue(app.textFields["settings-server"].waitForExistence(timeout: 5))
        app.textFields["settings-server"].tap()
        app.textFields["settings-server"].typeText("ws://signal.example.org/ws\n")
        shot("5-settings")
    }

    /**
     * The on-screen pad keeps working after every rotation (debug pad lab,
     * no room needed): each button sends its own bit and then 0, in
     * portrait (Game Boy) and in both landscape sides (Switch).
     */
    func testPadAcrossRotations() {
        let app = launch(["-padLab"])
        defer { XCUIDevice.shared.orientation = .portrait }
        let layout = app.staticTexts["pad-layout"]
        let log = app.staticTexts["pad-log"]
        XCTAssertTrue(layout.waitForExistence(timeout: 10))

        func expectPress(_ id: String, _ bits: Int, at offset: CGVector? = nil, _ where_: String) {
            app.buttons["pad-clear"].tap()
            XCTAssertEqual(log.label, "", "log not cleared (\(where_))")
            let el = app.otherElements[id]
            XCTAssertTrue(el.waitForExistence(timeout: 5), "\(id) missing (\(where_))")
            if let offset {
                el.coordinate(withNormalizedOffset: offset).press(forDuration: 0.3)
            } else {
                el.press(forDuration: 0.3)
            }
            let want = "\(bits) 0"
            let done = NSPredicate(format: "label == %@", want)
            let ok = XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: done, object: log)], timeout: 3) == .completed
            XCTAssertTrue(ok, "\(id) in \(where_): sent [\(log.label)], want [\(want)]")
        }

        let steps: [(UIDeviceOrientation, String)] = [
            (.portrait, "portrait"), (.landscapeLeft, "landscape"), (.portrait, "portrait"),
            (.landscapeRight, "landscape"), (.landscapeLeft, "landscape"), (.portrait, "portrait"),
        ]
        for (i, (orientation, name)) in steps.enumerated() {
            XCUIDevice.shared.orientation = orientation
            let turned = NSPredicate(format: "label == %@", name)
            XCTAssertEqual(XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: turned, object: layout)], timeout: 5), .completed, "never turned \(name)")
            sleep(1) // let the rotation animation finish
            let where_ = "step \(i) \(name)"
            shot("pad-\(i)-\(name)")
            expectPress("pad-button-1", 1 << 4, where_)
            expectPress("pad-button-6", 1 << 9, where_)
            expectPress("pad-coin", 1 << 11, where_)
            expectPress("pad-start-1", 1 << 18, where_)
            expectPress("pad-start-2", 1 << 19, where_)
            expectPress("pad-dpad", 1 << 3, at: CGVector(dx: 0.92, dy: 0.5), where_)
            expectPress("pad-dpad", 1 << 0, at: CGVector(dx: 0.5, dy: 0.08), where_)
        }
    }

    /**
     * The gamepad skins (debug pad lab with -labSkin): every control sends
     * its own bit in portrait and landscape, and the room's menu over the
     * picture hides after 3 s leaving its handle. With GL_SHOTS it saves
     * each skin in both orientations.
     */
    func testSkinPad() {
        defer { XCUIDevice.shared.orientation = .portrait }
        let skins: [(String, Int, Int)] = [("violet", 6, 2), ("blue", 4, 2), ("green", 2, 4), ("orange", 3, 1), ("red", 6, 4), ("smoke", 1, 2)]
        for (skin, buttons, starts) in skins {
            XCUIDevice.shared.orientation = .portrait
            let app = launch(["-padLab", "-labSkin", skin, "-labButtons", "\(buttons)", "-labStarts", "\(starts)"])
            let layout = app.staticTexts["pad-layout"]
            let log = app.staticTexts["pad-log"]
            XCTAssertTrue(layout.waitForExistence(timeout: 10), skin)
            for (orientation, name) in [(UIDeviceOrientation.portrait, "portrait"), (.landscapeLeft, "landscape")] {
                XCUIDevice.shared.orientation = orientation
                let turned = NSPredicate(format: "label == %@", name)
                XCTAssertEqual(XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: turned, object: layout)], timeout: 5), .completed, "never turned \(name)")
                sleep(1)
                shot("skin-\(skin)-\(name)")
                func expectPress(_ id: String, _ bits: Int, at offset: CGVector? = nil) {
                    app.buttons["pad-clear"].tap()
                    // An empty log may leave the accessibility tree.
                    let deadline = Date().addingTimeInterval(3)
                    while log.exists && !log.label.isEmpty && Date() < deadline { usleep(100_000) }
                    XCTAssertTrue(!log.exists || log.label.isEmpty, "log not cleared (\(skin) \(name)): [\(log.label)]")
                    let el = app.otherElements[id]
                    XCTAssertTrue(el.waitForExistence(timeout: 5), "\(id) missing (\(skin) \(name))")
                    if let offset { el.coordinate(withNormalizedOffset: offset).press(forDuration: 0.3) } else { el.press(forDuration: 0.3) }
                    let want = "\(bits) 0"
                    let ok = XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: NSPredicate(format: "label == %@", want), object: log)], timeout: 3) == .completed
                    XCTAssertTrue(ok, "\(id) in \(skin) \(name): sent [\(log.label)], want [\(want)]")
                }
                expectPress("pad-button-1", 1 << 4)
                expectPress("pad-button-\(buttons)", 1 << (3 + buttons))
                expectPress("pad-coin", 1 << 11)
                expectPress("pad-start-\(starts)", 1 << (17 + starts))
                expectPress("pad-dpad", 1 << 3, at: CGVector(dx: 0.85, dy: 0.5))
                expectPress("pad-dpad", 1 << 0, at: CGVector(dx: 0.5, dy: 0.15))
            }
            // In landscape the menu over the picture folds away into its handle (in portrait it sits under the picture and stays).
            XCTAssertTrue(app.buttons["dock-handle"].waitForExistence(timeout: 6), "\(skin): menu never hid")
            if skin == "violet" { shot("skin-violet-landscape-menu-hidden") }
            XCUIDevice.shared.orientation = .portrait
            sleep(4)
            XCTAssertFalse(app.buttons["dock-handle"].exists, "\(skin): the portrait menu must stay")
            app.terminate()
        }
    }

    /**
     * Settings › Install skin: a broken paste shows the error; a valid skin
     * shows its preview in both orientations, installs as "Custom · <name>",
     * can be chosen (the room's Game settings shows it selected) and deleted.
     */
    func testInstallSkinByPasting() {
        var app = launch(["-resetState"])
        XCTAssertTrue(app.buttons["home-settings"].waitForExistence(timeout: 10))
        app.buttons["home-settings"].tap()
        let open = app.buttons["settings-install-skin"]
        for _ in 0..<6 where !open.isHittable { app.swipeUp() }
        open.tap()
        let text = app.textViews["skin-install-text"]
        XCTAssertTrue(text.waitForExistence(timeout: 5))
        text.tap()
        text.typeText("{ \"format\": 1, oops")
        app.buttons["skin-install-check"].tap()
        XCTAssertTrue(app.staticTexts["skin-install-error"].waitForExistence(timeout: 3), "a broken paste explains itself")
        shot("install-skin-error")
        // Tap below the text, so the cursor lands at its end, and delete it.
        text.coordinate(withNormalizedOffset: CGVector(dx: 0.9, dy: 0.9)).tap()
        text.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: 30))
        text.typeText(##"{"format":1,"id":"ui-test","name":{"en":"UI Test"},"author":"tests","shell":{"center":"#2a6f97","edge":"#0b2233","rim":"#e0f0ff"}}"##)
        app.buttons["skin-install-check"].tap()
        let preview = app.descendants(matching: .any)["skin-install-preview"]
        let found = preview.waitForExistence(timeout: 5)
        if !found { shot("install-skin-no-preview") }
        XCTAssertTrue(found, "a valid skin shows its preview")
        XCTAssertTrue(app.staticTexts["skin-install-name"].label.contains("UI Test"))
        shot("install-skin-preview-portrait")
        app.segmentedControls["skin-install-orientation"].buttons.element(boundBy: 1).tap()
        shot("install-skin-preview-landscape")
        let install = app.buttons["skin-install-install"]
        for _ in 0..<4 where !install.isHittable { app.swipeUp() }
        install.tap()
        // A skin left by an earlier run makes the app ask before replacing it; then it offers to use it now.
        let replace = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Replace", "Reemplazar", "Substituir"])).firstMatch
        let useNow = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Use it now", "Usarla ahora", "Usar agora"])).firstMatch
        let deadline = Date().addingTimeInterval(15)
        while !useNow.exists && Date() < deadline {
            if replace.exists { replace.tap() }
            usleep(200_000)
        }
        if !useNow.exists { shot("install-skin-no-offer") }
        XCTAssertTrue(useNow.exists, "installing offers to use it now")
        useNow.tap()
        let installed = app.otherElements["skin-installed-ui-test"]
        for _ in 0..<4 where !installed.exists { app.swipeUp() }
        XCTAssertTrue(installed.waitForExistence(timeout: 5), "the installed list shows it")
        shot("install-skin-installed")
        app.terminate()

        // The choice holds in a room: the pad lab draws the chosen skin and Game settings marks it.
        app = launch(["-padLab", "-labSkin", "none"])
        XCTAssertTrue(app.buttons["lab-settings"].waitForExistence(timeout: 10))
        shot("install-skin-in-room")
        app.buttons["lab-settings"].tap()
        let tile = app.buttons["skin-ui-test"]
        for _ in 0..<6 where !tile.isHittable { app.swipeUp() }
        XCTAssertTrue(tile.waitForExistence(timeout: 5))
        XCTAssertTrue(tile.isSelected, "the pasted skin is the chosen one")
        XCTAssertTrue(tile.label.contains("UI Test"))
        shot("install-skin-game-settings")
        // Delete it from Install skin's list of installed skins, with a confirmation.
        let installButton = app.buttons["skin-install"]
        for _ in 0..<4 where !installButton.isHittable { app.swipeUp() }
        installButton.tap()
        let trash = app.buttons["skin-delete-ui-test"]
        for _ in 0..<4 where !trash.isHittable { app.swipeUp() }
        XCTAssertTrue(trash.waitForExistence(timeout: 5))
        trash.tap()
        let confirm = app.alerts.buttons.matching(NSPredicate(format: "label IN %@", ["Delete", "Borrar", "Apagar"])).firstMatch
        XCTAssertTrue(confirm.waitForExistence(timeout: 5))
        confirm.tap()
        XCTAssertFalse(app.buttons["skin-delete-ui-test"].waitForExistence(timeout: 2), "the skin is gone")
        app.buttons["back"].tap()
        XCTAssertFalse(app.buttons["skin-ui-test"].waitForExistence(timeout: 2), "the skin is gone")
        app.terminate()
    }

    /**
     * Landscape without the on-screen pad (a controller in hand): the
     * picture fills the height and the room's buttons float in a capsule
     * that folds into its handle, and a tap on the handle brings it back.
     */
    func testCinemaLayout() {
        defer { XCUIDevice.shared.orientation = .portrait }
        let app = launch(["-padLab", "-labNoPad"])
        XCUIDevice.shared.orientation = .landscapeLeft
        let layout = app.staticTexts["pad-layout"]
        XCTAssertEqual(XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: NSPredicate(format: "label == 'landscape'"), object: layout)], timeout: 5), .completed)
        XCTAssertTrue(app.buttons["dock-handle"].waitForExistence(timeout: 8), "the capsule folds away")
        shot("cinema-folded")
        app.buttons["dock-handle"].tap()
        XCTAssertTrue(app.buttons["lab-settings"].waitForExistence(timeout: 3), "the handle brings it back")
        shot("cinema-menu")
    }

    func testLiveJoin() throws {
        guard let invite = env["GL_INVITE"], let pinText = env["GL_PIN"] else { throw XCTSkip("no invitation given") }
        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let app = launch(["-invite", invite, "-silentOutput"])
        let pin = app.textFields["join-pin"]
        XCTAssertTrue(pin.waitForExistence(timeout: 10))
        if !pin.hasFocus { pin.tap() }
        pin.typeText(pinText)
        let terms = app.buttons["terms-check"]
        if !terms.isSelected { terms.tap() }
        shot("live-1-pin")
        app.buttons["join-button"].tap()

        // The name step comes once the PIN is accepted.
        let aliasEnter = app.buttons["alias-enter"]
        XCTAssertTrue(aliasEnter.waitForExistence(timeout: 40), "no name step")
        let aliasField = app.textFields["alias-field"]
        if !aliasEnter.isEnabled {
            aliasField.tap()
            aliasField.typeText("iPhone")
        }
        shot("live-1b-alias")
        // The keyboard may cover the button: Return enters too.
        if aliasEnter.isHittable { aliasEnter.tap() } else { aliasField.typeText("\n") }

        // Streaming: the seat line appears and the progress goes away.
        let seat = app.staticTexts["room-seat"]
        XCTAssertTrue(seat.waitForExistence(timeout: 40), "never got a seat")
        let progress = app.staticTexts["room-progress"]
        let gone = NSPredicate(format: "exists == false")
        expectation(for: gone, evaluatedWith: progress)
        waitForExpectations(timeout: 30)
        // No microphone prompt just for joining and listening.
        XCTAssertFalse(springboard.alerts.firstMatch.waitForExistence(timeout: 3), "a system alert appeared while joining")
        sleep(3)
        shot("live-2-room-portrait")

        // The picture styles on the live game (Game settings from the dock's gear).
        app.buttons["dock-settings"].tap()
        XCTAssertTrue(app.buttons["picture-style-crt"].waitForExistence(timeout: 5))
        app.buttons["picture-style-crt"].tap()
        reveal(app.buttons["picture-bands-frame"], in: app)
        app.buttons["picture-bands-frame"].tap()
        shot("live-2b-settings")
        app.buttons["game-settings-close"].tap()
        sleep(2)
        shot("live-2c-crt-frame")
        app.buttons["dock-settings"].tap()
        let compare = app.switches["picture-compare"]
        XCTAssertTrue(compare.waitForExistence(timeout: 5))
        reveal(compare, in: app)
        compare.coordinate(withNormalizedOffset: CGVector(dx: 0.95, dy: 0.5)).tap()
        app.buttons["game-settings-close"].tap()
        sleep(2)
        shot("live-2d-compare")
        app.buttons["dock-settings"].tap()
        XCTAssertTrue(compare.waitForExistence(timeout: 5))
        reveal(compare, in: app)
        compare.coordinate(withNormalizedOffset: CGVector(dx: 0.95, dy: 0.5)).tap()
        reveal(app.buttons["picture-style-sharp"], in: app)
        app.buttons["picture-style-sharp"].tap()
        reveal(app.buttons["picture-bands-ambient"], in: app)
        app.buttons["picture-bands-ambient"].tap()
        app.buttons["game-settings-close"].tap()

        // The on-screen pad: coin, start, a button, the D-pad.
        if !app.otherElements["pad-coin"].waitForExistence(timeout: 2) { app.buttons["dock-pad"].tap() }
        app.otherElements["pad-coin"].press(forDuration: 0.3)
        app.otherElements["pad-start-1"].press(forDuration: 0.3)
        app.otherElements["pad-button-1"].press(forDuration: 0.5)
        let dpad = app.otherElements["pad-dpad"]
        dpad.coordinate(withNormalizedOffset: CGVector(dx: 0.9, dy: 0.5)).press(forDuration: 0.5)

        // Chat.
        app.buttons["dock-chat"].tap()
        let input = app.textFields["chat-input"]
        XCTAssertTrue(input.waitForExistence(timeout: 5))
        input.tap()
        input.typeText("hello from the iOS app")
        app.buttons["chat-send"].tap()
        sleep(2)
        shot("live-3-chat")
        XCTAssertTrue(app.staticTexts["hello from the iOS app"].waitForExistence(timeout: 10))
        app.buttons["drawer-players"].tap()
        XCTAssertTrue(app.otherElements["seat-1"].waitForExistence(timeout: 5) || app.staticTexts["P1"].exists)
        shot("live-4-players")
        app.buttons["drawer-you"].tap()
        XCTAssertTrue(app.buttons["you-sound"].waitForExistence(timeout: 5))
        shot("live-5-you")
        app.buttons["you-sound"].tap()
        XCTAssertTrue(app.buttons["picture-style-crt"].waitForExistence(timeout: 5))
        shot("live-6-settings")
        app.buttons["picture-style-crt"].tap()
        sleep(2)
        shot("live-6b-crt")
        app.swipeUp()
        XCTAssertTrue(app.buttons["sound-test"].waitForExistence(timeout: 5))
        app.buttons["game-settings-close"].tap()
        sleep(1)
        if app.buttons["drawer-close"].exists { app.buttons["drawer-close"].tap() }

        // The microphone: our explanation, then the system's prompt (or straight on when allowed).
        let micOffLabel = app.buttons["dock-mic"].label
        app.buttons["dock-mic"].tap()
        let allow = app.alerts.buttons.element(boundBy: 1)
        if allow.waitForExistence(timeout: 3) {
            shot("live-7-mic-explainer")
            allow.tap()
            let system = springboard.alerts.firstMatch
            if system.waitForExistence(timeout: 5) {
                shot("live-7b-mic-system")
                let ok = system.buttons.allElementsBoundByIndex.last
                ok?.tap()
            }
        }
        let micOn = NSPredicate(format: "label != %@", micOffLabel)
        expectation(for: micOn, evaluatedWith: app.buttons["dock-mic"])
        waitForExpectations(timeout: 10)
        sleep(4)

        XCUIDevice.shared.orientation = .landscapeLeft
        sleep(3)
        shot("live-8-room-landscape")
        app.otherElements["pad-button-2"].press(forDuration: 0.5)
        XCUIDevice.shared.orientation = .portrait
        sleep(2)
        app.buttons["room-leave"].tap()
        XCTAssertTrue(app.buttons["home-scan"].waitForExistence(timeout: 10))
    }

    /**
     * The name step's validation (debug -aliasLab, no room): symbols and
     * emoji turn it red and disable "Enter the room"; a plain name enables it.
     */
    func testAliasValidation() {
        let app = launch(["-aliasLab"])
        let field = app.textFields["alias-field"]
        let enter = app.buttons["alias-enter"]
        let hint = app.staticTexts["alias-hint"]
        XCTAssertTrue(field.waitForExistence(timeout: 10))
        XCTAssertFalse(enter.isEnabled, "empty name")
        field.tap()
        field.typeText("Ana😀")
        XCTAssertFalse(enter.isEnabled, "emoji accepted")
        XCTAssertTrue(hint.label.contains(lang == "en" ? "no symbols or emoji" : (lang == "es" ? "sin símbolos" : "sem símbolos")), hint.label)
        shot("alias-invalid")
        field.clearAndType("Ana")
        XCTAssertTrue(enter.waitForEnabled(), "a plain name is refused")
        shot("alias-valid")
        field.typeText(" Ñandú 17")
        XCTAssertTrue(enter.isEnabled, "accents and ñ refused")
        enter.tap()
        XCTAssertEqual(app.staticTexts["alias-result"].label, "Ana Ñandú 17")
        field.clearAndType("A")
        XCTAssertFalse(enter.isEnabled, "one letter accepted")
    }

    /** Home opens the offline controller test, in portrait and landscape, and the on-screen pad lights it. */
    func testControllerTestScreen() {
        let app = launch()
        defer { XCUIDevice.shared.orientation = .portrait }
        let open = app.buttons["home-test-controller"]
        XCTAssertTrue(open.waitForExistence(timeout: 10))
        open.tap()
        XCTAssertTrue(app.otherElements["test-controller-screen"].waitForExistence(timeout: 5) || app.buttons["test-close"].exists)
        XCTAssertTrue(app.staticTexts["test-latency"].waitForExistence(timeout: 5))
        app.otherElements["pad-button-1"].press(forDuration: 0.4)
        app.otherElements["pad-dpad"].coordinate(withNormalizedOffset: CGVector(dx: 0.92, dy: 0.5)).press(forDuration: 0.4)
        // A press was measured: the last reading is a number, not a dash.
        let measured = NSPredicate(format: "NOT (label BEGINSWITH %@)", app.staticTexts["test-latency"].label.components(separatedBy: " ").prefix(2).joined(separator: " ") + " –")
        XCTAssertEqual(XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: measured, object: app.staticTexts["test-latency"])], timeout: 3), .completed, app.staticTexts["test-latency"].label)
        // The screen's refresh rate is measured within a couple of seconds ("… 60 Hz …").
        let rate = app.staticTexts["test-screen-rate"]
        XCTAssertEqual(XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: NSPredicate(format: "label CONTAINS ' Hz'"), object: rate)], timeout: 5), .completed, rate.label)
        shot("test-controller-portrait")
        XCUIDevice.shared.orientation = .landscapeLeft
        sleep(2)
        XCTAssertTrue(app.otherElements["pad-button-6"].waitForExistence(timeout: 5))
        app.otherElements["pad-button-6"].press(forDuration: 0.4)
        shot("test-controller-landscape")
        XCUIDevice.shared.orientation = .portrait
        sleep(1)
        app.buttons["test-close"].tap()
        XCTAssertTrue(app.buttons["home-scan"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["app-version"].exists)
    }

    /**
     * Game settings from the dock's gear (debug -pictureLab: the test card
     * streamed at 60 fps into the real renderer): switch the picture style
     * and sides, turn Compare on (the divider appears), in portrait and
     * landscape. With GL_SHOTS it saves every style and side.
     */
    func testGameSettingsAndPictureStyles() {
        let app = launch(["-pictureLab"])
        defer { XCUIDevice.shared.orientation = .portrait }
        let gear = app.buttons["dock-settings"]
        XCTAssertTrue(gear.waitForExistence(timeout: 10))
        // The test card really is drawn (about 60 new frames a second).
        let fps = app.staticTexts["picture-lab-fps"]
        let drawing = NSPredicate(format: "NOT (label BEGINSWITH '0 fps')")
        XCTAssertEqual(XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: drawing, object: fps)], timeout: 5), .completed, fps.label)
        for orientation in [UIDeviceOrientation.portrait, .landscapeLeft] {
            XCUIDevice.shared.orientation = orientation
            sleep(1)
            let side = orientation == .portrait ? "portrait" : "landscape"
            gear.tap()
            XCTAssertTrue(app.buttons["picture-style-sharp"].waitForExistence(timeout: 5), "Game settings did not open")
            shot("picture-settings-\(side)")
            for style in ["smooth", "sharp", "crt", "edges"] {
                let b = app.buttons["picture-style-\(style)"]
                pick(b, in: app)
                XCTAssertTrue(b.isSelected, "\(style) not selected")
            }
            pick(app.buttons["picture-style-sharp"], in: app)
            let ambient = app.buttons["picture-bands-ambient"]
            reveal(ambient, in: app)
            ambient.tap()
            XCTAssertTrue(ambient.isSelected)
            app.buttons["game-settings-close"].tap()
            XCTAssertFalse(app.buttons["picture-style-sharp"].waitForExistence(timeout: 1))
            if env["GL_SHOTS"] != nil {
                for style in ["smooth", "sharp", "crt", "edges"] {
                    for bands in ["black", "ambient", "frame"] {
                        gear.tap()
                        _ = app.buttons["picture-style-\(style)"].waitForExistence(timeout: 3)
                        pick(app.buttons["picture-style-\(style)"], in: app)
                        let bb = app.buttons["picture-bands-\(bands)"]
                        reveal(bb, in: app)
                        bb.tap()
                        app.buttons["game-settings-close"].tap()
                        sleep(1)
                        shot("picture-\(side)-\(style)-\(bands)")
                    }
                }
            }
            // Compare: the divider over the picture.
            gear.tap()
            _ = app.buttons["picture-style-crt"].waitForExistence(timeout: 3)
            pick(app.buttons["picture-style-crt"], in: app)
            let compare = app.switches["picture-compare"]
            sleep(1) // the panel slides in
            reveal(compare, in: app)
            if (compare.value as? String) != "1" { compare.coordinate(withNormalizedOffset: CGVector(dx: 0.95, dy: 0.5)).tap() }
            let on = XCTNSPredicateExpectation(predicate: NSPredicate(format: "value == '1'"), object: compare)
            XCTAssertEqual(XCTWaiter().wait(for: [on], timeout: 3), .completed, "\(compare.value ?? "nil") shots=\(env["GL_SHOTS"] ?? "none") \(compare.frame) \(app.frame)")
            app.buttons["game-settings-close"].tap()
            let divider = app.otherElements["picture-divider"]
            XCTAssertTrue(divider.waitForExistence(timeout: 3) || app.descendants(matching: .any)["picture-divider"].exists, "no divider")
            sleep(1)
            shot("picture-\(side)-compare")
            gear.tap()
            _ = compare.waitForExistence(timeout: 3)
            sleep(1) // the panel slides in
            reveal(compare, in: app)
            if (compare.value as? String) == "1" { compare.coordinate(withNormalizedOffset: CGVector(dx: 0.95, dy: 0.5)).tap() }
            if app.buttons["game-settings-close"].exists { app.buttons["game-settings-close"].tap() }
        }
    }

    /** A room default from the host applies until the viewer picks their own, and "Use the room's default" brings it back. */
    func testPictureRoomDefault() {
        let app = launch(["-pictureLab", "-labRoom", "crt,frame", "-labSettings"])
        let crt = app.buttons["picture-style-crt"]
        XCTAssertTrue(crt.waitForExistence(timeout: 10))
        XCTAssertTrue(crt.isSelected, "the room's default style is not in use")
        XCTAssertTrue(app.buttons["picture-bands-frame"].isSelected)
        XCTAssertTrue(app.staticTexts["picture-room-default"].exists)
        XCTAssertFalse(app.buttons["picture-use-room-default"].exists)
        app.buttons["picture-style-sharp"].tap()
        XCTAssertTrue(app.buttons["picture-style-sharp"].isSelected)
        let back = app.buttons["picture-use-room-default"]
        XCTAssertTrue(back.waitForExistence(timeout: 3))
        shot("picture-room-default")
        back.tap()
        XCTAssertTrue(crt.isSelected)
        XCTAssertFalse(back.waitForExistence(timeout: 1))
    }

    /**
     * The 2x stream path (docs/protocol.md, Video scale): offscreen, the test
     * card sent 2x with nearest neighbour and averaged back must be byte for
     * byte the native one in every style and side (-picture2xCheck). Then
     * the live lab streams 2x frames (-labUp2) through the real view.
     */
    func testPicture2xMatchesNative() {
        let app = launch(["-picture2xCheck"])
        let label = app.staticTexts["picture-2x-check"]
        XCTAssertTrue(label.waitForExistence(timeout: 10))
        let done = XCTNSPredicateExpectation(predicate: NSPredicate(format: "NOT (label BEGINSWITH 'running')"), object: label)
        XCTAssertEqual(XCTWaiter().wait(for: [done], timeout: 180), .completed, label.label)
        shot("picture-2x-check")
        XCTAssertTrue(label.label.hasPrefix("ok "), label.label)
        app.terminate()
        for style in ["crt", "edges"] {
            let lab = launch(["-pictureLab", "-labUp2", "-labStyle", style, "-labBands", "ambient"])
            let fps = lab.staticTexts["picture-lab-fps"]
            XCTAssertTrue(fps.waitForExistence(timeout: 10))
            let drawing = NSPredicate(format: "NOT (label BEGINSWITH '0 fps')")
            XCTAssertEqual(XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: drawing, object: fps)], timeout: 5), .completed, fps.label)
            XCTAssertTrue(fps.label.hasSuffix("2×"), fps.label)
            sleep(1)
            shot("picture-2x-lab-\(style)")
            lab.terminate()
        }
    }

    /** Screenshots of the pad lab with the stats overlay and the see-through pad of a controller. */
    func testStatsAndGhostPadLab() {
        let app = launch(["-padLab", "-labStats", "-labGhost"])
        defer { XCUIDevice.shared.orientation = .portrait }
        XCTAssertTrue(app.staticTexts["pad-layout"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.otherElements["room-stats-box"].exists || app.staticTexts["room-stats-box"].exists)
        XCTAssertTrue(app.buttons["pad-controller-chip"].exists)
        // Display only: a finger on the pad sends nothing.
        app.otherElements["pad-button-2"].press(forDuration: 0.3)
        XCTAssertEqual(app.staticTexts["pad-log"].label, "")
        sleep(2) // the stats' screen line gets its first measured rate
        shot("lab-stats-ghost-portrait")
        XCUIDevice.shared.orientation = .landscapeLeft
        sleep(2)
        shot("lab-stats-ghost-landscape")
    }
}

/** Scrolls Game settings until the element is fully on screen (isHittable is not enough in a scroll view). */
private func reveal(_ el: XCUIElement, in app: XCUIApplication) {
    let panel = app.otherElements["game-settings"].scrollViews.firstMatch
    for _ in 0..<6 {
        let f = el.frame
        let screen = app.frame
        if f.minY >= screen.minY + 60 && f.maxY <= screen.maxY - 20 { return }
        let target = panel.exists ? panel : app
        if f.maxY > screen.maxY - 20 { target.swipeUp(velocity: .slow) } else { target.swipeDown(velocity: .slow) }
    }
}

/** Reveals and taps an option of Game settings (a tap outside the panel would close it). */
private func pick(_ el: XCUIElement, in app: XCUIApplication) {
    _ = el.waitForExistence(timeout: 3)
    reveal(el, in: app)
    el.tap()
}

extension XCUIElement {
    func clearAndType(_ text: String) {
        tap()
        let current = value as? String ?? ""
        typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count + 4) + text)
    }

    func waitForEnabled(timeout: TimeInterval = 3) -> Bool {
        XCTWaiter().wait(for: [XCTNSPredicateExpectation(predicate: NSPredicate(format: "isEnabled == true"), object: self)], timeout: timeout) == .completed
    }
}
