// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import org.junit.Assert.assertEquals
import org.junit.Test

class CodeInputTest {
    private fun type(text: String, caret: Int, s: String) = CodeInput.edit(text, caret, caret, s)

    @Test
    fun typingDigitByDigitGroupsThree() {
        var edit = CodeInput.Edit("", 0)
        for (d in "915355636") edit = type(edit.text, edit.caret, d.toString())
        assertEquals(CodeInput.Edit("915 355 636", 11), edit)
        assertEquals(CodeInput.Edit("915", 3), type("91", 2, "5"))
        assertEquals(CodeInput.Edit("915 3", 5), type("915", 3, "3"))
    }

    @Test
    fun typingInTheMiddleKeepsTheCaretAfterTheNewDigit() {
        // "915 |355" + 0 -> "915 035 5", caret after the 0.
        assertEquals(CodeInput.Edit("915 035 5", 5), type("915 355", 4, "0"))
        assertEquals(CodeInput.Edit("910 535 5", 3), type("915 355", 2, "0"))
    }

    @Test
    fun backspaceAcrossASpaceDeletesTheDigitBefore() {
        // "915 |355": the backspace removes the space [3,4) and so the 5.
        assertEquals(CodeInput.Edit("913 55", 2), CodeInput.edit("915 355", 3, 4, ""))
        assertEquals(CodeInput.Edit("913 55", 2), CodeInput.fromChange("915 355", "915355", 3))
        // A plain backspace after a digit.
        assertEquals(CodeInput.Edit("915 35", 6), CodeInput.fromChange("915 355", "915 35", 6))
    }

    @Test
    fun deletingASelection() {
        assertEquals(CodeInput.Edit("916 36", 2), CodeInput.edit("915 355 636", 2, 7, ""))
    }

    @Test
    fun pasteWithOrWithoutSeparators() {
        val want = CodeInput.Edit("915 355 636", 11)
        assertEquals(want, type("", 0, "915355636"))
        assertEquals(want, type("", 0, "915 355 636"))
        assertEquals(want, type("", 0, "915-355-636"))
        assertEquals(want, CodeInput.fromChange("", "915 355 636", 11))
    }

    @Test
    fun aLinkIsLeftAsTyped() {
        val link = "https://play.go-link.org/g/AbCdEfGhIjKlMnOpQrStUv"
        assertEquals(CodeInput.Edit(link, link.length), type("", 0, link))
        assertEquals(InviteTarget.Link("AbCdEfGhIjKlMnOpQrStUv"), Invites.parseTyped(type("", 0, link).text))
    }

    @Test
    fun moreThanNineDigitsAreCut() {
        assertEquals(CodeInput.Edit("123 456 789", 11), type("", 0, "1234567890123"))
        assertEquals(CodeInput.Edit("123 456 789", 11), type("123 456 789", 11, "0"))
    }

    @Test
    fun withoutLinksLettersAreDropped() {
        assertEquals(CodeInput.Edit("915 3", 5), CodeInput.edit("915", 3, 3, "a3b", allowLinks = false))
    }

    @Test
    fun theGroupedCodeStillParses() {
        assertEquals(InviteTarget.Code("915355636"), Invites.parseTyped("915 355 636"))
    }
}
