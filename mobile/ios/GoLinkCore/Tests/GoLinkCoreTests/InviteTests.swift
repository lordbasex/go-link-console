// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import GoLinkCore

final class InviteTests: XCTestCase {
    private let invite = "AbCdEfGhIjKlMnOpQr_-12"

    func testAcceptsTheInvitationLinkAndTheCode() {
        XCTAssertEqual(.link(invite), Invites.parseScanned("https://play.go-link.org/g/\(invite)"))
        XCTAssertEqual(.link(invite), Invites.parseScanned("  https://play.go-link.org/g/\(invite)/ \n"))
        XCTAssertEqual(.code("123456789"), Invites.parseScanned("123 456 789"))
        XCTAssertEqual(.code("123456789"), Invites.parseScanned("123-456-789"))
    }

    func testRejectsEverythingElseFromAQrCode() {
        let bad = [
            "",
            "http://play.go-link.org/g/\(invite)", // not https
            "https://evil.example/g/\(invite)",
            "https://play.go-link.org.evil.example/g/\(invite)",
            "https://www.go-link.org/g/\(invite)",
            "https://user@play.go-link.org/g/\(invite)",
            "https://play.go-link.org:8443/g/\(invite)",
            "https://play.go-link.org/g/\(invite)A", // 23 characters
            "https://play.go-link.org/g/\(invite.dropLast())",
            "https://play.go-link.org/g/\(invite)?server=wss://evil.example/ws",
            "https://play.go-link.org/g/\(invite)#pin=123456",
            "https://play.go-link.org/r/\(invite)",
            "wss://evil.example/ws",
            "go-link://g/\(invite)",
            invite, // a bare invite is only accepted when typed
            "12345678", // 8 digits
            "1234567890",
            "123456", // a PIN
            "https://play.go-link.org/g/\(String(repeating: "x", count: 300))",
            "１２３４５６７８９", // full-width digits
        ]
        for text in bad { XCTAssertNil(Invites.parseScanned(text), text) }
    }

    func testTypedTextAlsoTakesTheBareInvite() {
        XCTAssertEqual(.link(invite), Invites.parseTyped(invite))
        XCTAssertEqual(.code("987654321"), Invites.parseTyped("987.654.321"))
        XCTAssertEqual(.link(invite), Invites.parseTyped("https://play.go-link.org/g/\(invite)"))
        XCTAssertNil(Invites.parseTyped("https://evil.example/g/\(invite)"))
        XCTAssertNil(Invites.parseTyped("hello"))
    }

    func testUniversalLinks() {
        XCTAssertEqual(.link(invite), Invites.parseAppLink(scheme: "https", host: "play.go-link.org", path: "/g/\(invite)"))
        XCTAssertNil(Invites.parseAppLink(scheme: "http", host: "play.go-link.org", path: "/g/\(invite)"))
        XCTAssertNil(Invites.parseAppLink(scheme: "https", host: "evil.example", path: "/g/\(invite)"))
        XCTAssertNil(Invites.parseAppLink(scheme: "https", host: "play.go-link.org", path: "/g/\(invite)/extra"))
        XCTAssertNil(Invites.parseAppLink(scheme: "https", host: "play.go-link.org", path: nil))
        // From a URL: query and fragment ignored, user info and ports refused.
        XCTAssertEqual(.link(invite), Invites.parseAppLink(URL(string: "https://play.go-link.org/g/\(invite)?pin=123456#x")!))
        XCTAssertNil(Invites.parseAppLink(URL(string: "https://user@play.go-link.org/g/\(invite)")!))
        XCTAssertNil(Invites.parseAppLink(URL(string: "https://play.go-link.org:8443/g/\(invite)")!))
    }

    func testPins() {
        XCTAssertTrue(Invites.isPin("012345"))
        XCTAssertFalse(Invites.isPin("12345"))
        XCTAssertFalse(Invites.isPin("12345a"))
        XCTAssertFalse(Invites.isPin("١٢٣٤٥٦")) // Arabic-Indic digits
        XCTAssertEqual("123 456 789", InviteTarget.code("123456789").display)
    }
}
