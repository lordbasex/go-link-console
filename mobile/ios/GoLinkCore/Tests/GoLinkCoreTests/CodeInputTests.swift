// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

@testable import GoLinkCore
import XCTest

final class CodeInputTests: XCTestCase {
    private func type(_ s: String) -> (text: String, caret: Int) {
        var state = (text: "", caret: 0)
        for ch in s { state = CodeInput.edit(text: state.text, start: state.caret, end: state.caret, insert: String(ch)) }
        return state
    }

    func testGroup() {
        XCTAssertEqual(CodeInput.group(""), "")
        XCTAssertEqual(CodeInput.group("915"), "915")
        XCTAssertEqual(CodeInput.group("9153"), "915 3")
        XCTAssertEqual(CodeInput.group("915355636"), "915 355 636")
    }

    func testTypingDigitByDigit() {
        XCTAssertEqual(type("915").text, "915")
        XCTAssertEqual(type("915").caret, 3)
        let four = type("9153")
        XCTAssertEqual(four.text, "915 3")
        XCTAssertEqual(four.caret, 5)
        let all = type("915355636")
        XCTAssertEqual(all.text, "915 355 636")
        XCTAssertEqual(all.caret, 11)
    }

    func testTypingInTheMiddle() {
        // "915 |355 63" + "0" -> "915 035 563"
        let r = CodeInput.edit(text: "915 355 63", start: 4, end: 4, insert: "0")
        XCTAssertEqual(r.text, "915 035 563")
        XCTAssertEqual(r.caret, 5)
    }

    func testBackspaceAcrossASpace() {
        // "915 |355": backspace removes the space, and with it the 5.
        let r = CodeInput.edit(text: "915 355", start: 3, end: 4, insert: "")
        XCTAssertEqual(r.text, "913 55")
        XCTAssertEqual(r.caret, 2)
    }

    func testBackspaceOnADigit() {
        let r = CodeInput.edit(text: "915 3", start: 4, end: 5, insert: "")
        XCTAssertEqual(r.text, "915")
        XCTAssertEqual(r.caret, 3)
    }

    func testDeletingASelection() {
        let r = CodeInput.edit(text: "915 355 636", start: 2, end: 6, insert: "")
        XCTAssertEqual(r.text, "915 636")
        XCTAssertEqual(r.caret, 2)
    }

    func testPasteWithAndWithoutSpaces() {
        for paste in ["915355636", "915 355 636", "915-355-636", " 915.355.636 "] {
            let r = CodeInput.edit(text: "", start: 0, end: 0, insert: paste)
            XCTAssertEqual(r.text, "915 355 636", paste)
            XCTAssertEqual(r.caret, 11, paste)
        }
    }

    func testPasteALinkIsLeftAlone() {
        let link = "https://play.go-link.org/g/AbCdEfGhIjKlMnOpQrStUv"
        let r = CodeInput.edit(text: "", start: 0, end: 0, insert: link)
        XCTAssertEqual(r.text, link)
        XCTAssertEqual(r.caret, link.count)
        XCTAssertEqual(Invites.parseTyped(r.text), .link("AbCdEfGhIjKlMnOpQrStUv"))
    }

    func testMoreThanNineDigitsAreCut() {
        let r = CodeInput.edit(text: "915 355 636", start: 11, end: 11, insert: "12")
        XCTAssertEqual(r.text, "915 355 636")
        XCTAssertEqual(r.caret, 11)
        XCTAssertEqual(CodeInput.edit(text: "", start: 0, end: 0, insert: "1234567890123").text, "123 456 789")
    }

    func testCodeOnlyFieldDropsLetters() {
        let r = CodeInput.edit(text: "915", start: 3, end: 3, insert: "a3b", allowLinks: false)
        XCTAssertEqual(r.text, "915 3")
        XCTAssertEqual(r.caret, 5)
    }

    func testGroupedTextParsesAsACode() {
        XCTAssertEqual(Invites.parseTyped("915 355 636"), .code("915355636"))
    }
}
