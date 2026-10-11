// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class InviteTest {
    private val invite = "AbCdEfGhIjKlMnOpQr_-12"

    @Test
    fun acceptsTheInvitationLinkAndTheCode() {
        assertEquals(InviteTarget.Link(invite), Invites.parseScanned("https://play.go-link.org/g/$invite"))
        assertEquals(InviteTarget.Link(invite), Invites.parseScanned("  https://play.go-link.org/g/$invite/ \n"))
        assertEquals(InviteTarget.Code("123456789"), Invites.parseScanned("123 456 789"))
        assertEquals(InviteTarget.Code("123456789"), Invites.parseScanned("123-456-789"))
    }

    @Test
    fun rejectsEverythingElseFromAQrCode() {
        val bad = listOf(
            "",
            "http://play.go-link.org/g/$invite", // not https
            "https://evil.example/g/$invite",
            "https://play.go-link.org.evil.example/g/$invite",
            "https://www.go-link.org/g/$invite",
            "https://user@play.go-link.org/g/$invite",
            "https://play.go-link.org:8443/g/$invite",
            "https://play.go-link.org/g/${invite}A", // 23 characters
            "https://play.go-link.org/g/${invite.dropLast(1)}",
            "https://play.go-link.org/g/$invite?server=wss://evil.example/ws",
            "https://play.go-link.org/g/$invite#pin=123456",
            "https://play.go-link.org/r/$invite",
            "wss://evil.example/ws",
            "go-link://g/$invite",
            invite, // a bare invite is only accepted when typed
            "12345678", // 8 digits
            "1234567890",
            "123456", // a PIN
            "https://play.go-link.org/g/${"x".repeat(300)}",
        )
        for (text in bad) assertNull(text, Invites.parseScanned(text))
    }

    @Test
    fun typedTextAlsoTakesTheBareInvite() {
        assertEquals(InviteTarget.Link(invite), Invites.parseTyped(invite))
        assertEquals(InviteTarget.Code("987654321"), Invites.parseTyped("987.654.321"))
        assertEquals(InviteTarget.Link(invite), Invites.parseTyped("https://play.go-link.org/g/$invite"))
        assertNull(Invites.parseTyped("https://evil.example/g/$invite"))
        assertNull(Invites.parseTyped("hello"))
    }

    @Test
    fun appLinks() {
        assertEquals(InviteTarget.Link(invite), Invites.parseAppLink("https", "play.go-link.org", "/g/$invite"))
        assertNull(Invites.parseAppLink("http", "play.go-link.org", "/g/$invite"))
        assertNull(Invites.parseAppLink("https", "evil.example", "/g/$invite"))
        assertNull(Invites.parseAppLink("https", "play.go-link.org", "/g/$invite/extra"))
        assertNull(Invites.parseAppLink("https", "play.go-link.org", null))
    }

    @Test
    fun pins() {
        assertEquals(true, Invites.isPin("012345"))
        assertEquals(false, Invites.isPin("12345"))
        assertEquals(false, Invites.isPin("12345a"))
        assertEquals("123 456 789", InviteTarget.Code("123456789").display())
    }
}
