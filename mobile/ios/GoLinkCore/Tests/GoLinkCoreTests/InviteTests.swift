// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import XCTest
@testable import GoLinkCore

final class InviteTests: XCTestCase {
    private let invite = "AbCdEfGhIjKlMnOpQr_-12"

    func testAcceptsTheInvitationLinkAndTheCode() {
        XCTAssertEqual(.link(invite), Invites.parseScanned("https://go-link.org/g/\(invite)"))
        XCTAssertEqual(.link(invite), Invites.parseScanned("  https://go-link.org/g/\(invite)/ \n"))
        XCTAssertEqual(.code("123456789"), Invites.parseScanned("123 456 789"))
        XCTAssertEqual(.code("123456789"), Invites.parseScanned("123-456-789"))
    }

    func testRejectsEverythingElseFromAQrCode() {
        let bad = [
            "",
            "http://go-link.org/g/\(invite)", // not https
            "https://evil.example/g/\(invite)",
            "https://go-link.org.evil.example/g/\(invite)",
            "https://www.go-link.org/g/\(invite)",
            "https://user@go-link.org/g/\(invite)",
            "https://go-link.org:8443/g/\(invite)",
            "https://go-link.org/g/\(invite)A", // 23 characters
            "https://go-link.org/g/\(invite.dropLast())",
            "https://go-link.org/g/\(invite)?server=wss://evil.example/ws",
            "https://go-link.org/g/\(invite)#pin=123456",
            "https://go-link.org/r/\(invite)",
            "wss://evil.example/ws",
            "go-link://g/\(invite)",
            invite, // a bare invite is only accepted when typed
            "12345678", // 8 digits
            "1234567890",
            "123456", // a PIN
            "https://go-link.org/g/\(String(repeating: "x", count: 300))",
            "１２３４５６７８９", // full-width digits
        ]
        for text in bad { XCTAssertNil(Invites.parseScanned(text), text) }
    }

    func testTypedTextAlsoTakesTheBareInvite() {
        XCTAssertEqual(.link(invite), Invites.parseTyped(invite))
        XCTAssertEqual(.code("987654321"), Invites.parseTyped("987.654.321"))
        XCTAssertEqual(.link(invite), Invites.parseTyped("https://go-link.org/g/\(invite)"))
        XCTAssertNil(Invites.parseTyped("https://evil.example/g/\(invite)"))
        XCTAssertNil(Invites.parseTyped("hello"))
    }

    func testUniversalLinks() {
        XCTAssertEqual(.link(invite), Invites.parseAppLink(scheme: "https", host: "go-link.org", path: "/g/\(invite)"))
        XCTAssertNil(Invites.parseAppLink(scheme: "http", host: "go-link.org", path: "/g/\(invite)"))
        XCTAssertNil(Invites.parseAppLink(scheme: "https", host: "evil.example", path: "/g/\(invite)"))
        XCTAssertNil(Invites.parseAppLink(scheme: "https", host: "go-link.org", path: "/g/\(invite)/extra"))
        XCTAssertNil(Invites.parseAppLink(scheme: "https", host: "go-link.org", path: nil))
        // From a URL: query and fragment ignored, user info and ports refused.
        XCTAssertEqual(.link(invite), Invites.parseAppLink(URL(string: "https://go-link.org/g/\(invite)?pin=123456#x")!))
        XCTAssertNil(Invites.parseAppLink(URL(string: "https://user@go-link.org/g/\(invite)")!))
        XCTAssertNil(Invites.parseAppLink(URL(string: "https://go-link.org:8443/g/\(invite)")!))
    }

    private let key = "K" + String(repeating: "e", count: 40) + "_-" // 43 characters

    func testGroupInvitationKeys() {
        // A QR code or a pasted link: exactly #k=<43 base64url characters>.
        XCTAssertEqual(Invitation(target: .link(invite), key: key), Invites.scanned("https://go-link.org/g/\(invite)#k=\(key)"))
        XCTAssertEqual(Invitation(target: .link(invite), key: key), Invites.typed(" https://go-link.org/g/\(invite)/#k=\(key)\n"))
        XCTAssertEqual(Invitation(target: .link(invite)), Invites.scanned("https://go-link.org/g/\(invite)"))
        XCTAssertEqual(Invitation(target: .code("123456789")), Invites.typed("123 456 789"))
        XCTAssertFalse(Invitation(target: .link(invite)).isGroup)
        // A key of the wrong length, another value or a query is not a group invitation.
        XCTAssertNil(Invites.scanned("https://go-link.org/g/\(invite)#k=\(key.dropLast())"))
        XCTAssertNil(Invites.scanned("https://go-link.org/g/\(invite)#k=\(key)A"))
        XCTAssertNil(Invites.scanned("https://go-link.org/g/\(invite)#k=\(key)&x=1"))
        XCTAssertNil(Invites.scanned("https://go-link.org/g/\(invite)?k=\(key)"))
        XCTAssertNil(Invites.scanned("https://evil.example/g/\(invite)#k=\(key)"))
        XCTAssertNil(Invites.typed("https://go-link.org/g/\(invite)#k=\(key.dropLast())!"))

        // The fragment of a Universal Link: k among other values, or nothing.
        XCTAssertEqual(key, Invites.groupKey(fragment: "k=\(key)"))
        XCTAssertEqual(key, Invites.groupKey(fragment: "#k=\(key)"))
        XCTAssertEqual(key, Invites.groupKey(fragment: "a=1&k=\(key)&b=2"))
        XCTAssertEqual("", Invites.groupKey(fragment: "k=\(key.dropLast())"))
        XCTAssertEqual("", Invites.groupKey(fragment: "k=\(key)x"))
        XCTAssertEqual("", Invites.groupKey(fragment: "kk=\(key)"))
        XCTAssertEqual("", Invites.groupKey(fragment: "pin=123456"))
        XCTAssertEqual("", Invites.groupKey(fragment: ""))

        let url = URL(string: "https://go-link.org/g/\(invite)#k=\(key)")!
        XCTAssertEqual(Invitation(target: .link(invite), key: key), Invites.appLink(url))
        XCTAssertEqual(Invitation(target: .link(invite), key: key), Invites.appLink(URL(string: "https://go-link.org/g/\(invite)?x=1#x=2&k=\(key)")!))
        // No fragment, a short key, or a key in the query: a one-person invitation.
        XCTAssertEqual(Invitation(target: .link(invite)), Invites.appLink(URL(string: "https://go-link.org/g/\(invite)")!))
        XCTAssertEqual(Invitation(target: .link(invite)), Invites.appLink(URL(string: "https://go-link.org/g/\(invite)#k=short")!))
        XCTAssertEqual(Invitation(target: .link(invite)), Invites.appLink(URL(string: "https://go-link.org/g/\(invite)?k=\(key)")!))
        XCTAssertNil(Invites.appLink(URL(string: "https://evil.example/g/\(invite)#k=\(key)")!))
        // The key never shows in a description (logs).
        XCTAssertFalse(String(describing: Invitation(target: .link(invite), key: key)).contains(key))
    }

    func testPins() {
        XCTAssertTrue(Invites.isPin("012345"))
        XCTAssertFalse(Invites.isPin("12345"))
        XCTAssertFalse(Invites.isPin("12345a"))
        XCTAssertFalse(Invites.isPin("١٢٣٤٥٦")) // Arabic-Indic digits
        XCTAssertEqual("123 456 789", InviteTarget.code("123456789").display)
    }
}
