// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class InviteTest {
    private val invite = "AbCdEfGhIjKlMnOpQr_-12"

    @Test
    fun acceptsTheInvitationLinkAndTheCode() {
        assertEquals(InviteTarget.Link(invite), Invites.parseScanned("https://go-link.org/g/$invite"))
        assertEquals(InviteTarget.Link(invite), Invites.parseScanned("  https://go-link.org/g/$invite/ \n"))
        assertEquals(InviteTarget.Code("123456789"), Invites.parseScanned("123 456 789"))
        assertEquals(InviteTarget.Code("123456789"), Invites.parseScanned("123-456-789"))
    }

    @Test
    fun rejectsEverythingElseFromAQrCode() {
        val bad = listOf(
            "",
            "http://go-link.org/g/$invite", // not https
            "https://evil.example/g/$invite",
            "https://go-link.org.evil.example/g/$invite",
            "https://www.go-link.org/g/$invite",
            "https://user@go-link.org/g/$invite",
            "https://go-link.org:8443/g/$invite",
            "https://go-link.org/g/${invite}A", // 23 characters
            "https://go-link.org/g/${invite.dropLast(1)}",
            "https://go-link.org/g/$invite?server=wss://evil.example/ws",
            "https://go-link.org/r/$invite",
            "wss://evil.example/ws",
            "go-link://g/$invite",
            invite, // a bare invite is only accepted when typed
            "12345678", // 8 digits
            "1234567890",
            "123456", // a PIN
            "https://go-link.org/g/${"x".repeat(300)}",
        )
        for (text in bad) assertNull(text, Invites.parseScanned(text))
    }

    @Test
    fun typedTextAlsoTakesTheBareInvite() {
        assertEquals(InviteTarget.Link(invite), Invites.parseTyped(invite))
        assertEquals(InviteTarget.Code("987654321"), Invites.parseTyped("987.654.321"))
        assertEquals(InviteTarget.Link(invite), Invites.parseTyped("https://go-link.org/g/$invite"))
        assertNull(Invites.parseTyped("https://evil.example/g/$invite"))
        assertNull(Invites.parseTyped("hello"))
    }

    @Test
    fun appLinks() {
        assertEquals(InviteTarget.Link(invite), Invites.parseAppLink("https", "go-link.org", "/g/$invite"))
        assertNull(Invites.parseAppLink("http", "go-link.org", "/g/$invite"))
        assertNull(Invites.parseAppLink("https", "evil.example", "/g/$invite"))
        assertNull(Invites.parseAppLink("https", "go-link.org", "/g/$invite/extra"))
        assertNull(Invites.parseAppLink("https", "go-link.org", null))
    }

    @Test
    fun groupInvitationKeys() {
        val key = "Ab_-".repeat(10) + "xyz" // 43 characters
        assertEquals(InviteTarget.Link(invite, key), Invites.parseScanned("https://go-link.org/g/$invite#k=$key"))
        assertEquals(InviteTarget.Link(invite, key), Invites.parseTyped("https://go-link.org/g/$invite/#k=$key"))
        assertEquals(InviteTarget.Link(invite, key), Invites.parseAppLink("https", "go-link.org", "/g/$invite", "k=$key"))
        // Other values in the fragment are ignored; k is still read.
        assertEquals(InviteTarget.Link(invite, key), Invites.parseScanned("https://go-link.org/g/$invite#x=1&k=$key&y=2"))
        // A key of the wrong length or with other characters is ignored: a plain invitation.
        for (bad in listOf("k=${key.dropLast(1)}", "k=${key}A", "k=${key.dropLast(1)}=", "key=$key", "K=$key", "pin=123456", "")) {
            assertEquals(bad, InviteTarget.Link(invite), Invites.parseScanned("https://go-link.org/g/$invite#$bad"))
            assertEquals(bad, InviteTarget.Link(invite), Invites.parseAppLink("https", "go-link.org", "/g/$invite", bad))
        }
        // No fragment: no key.
        assertEquals(InviteTarget.Link(invite), Invites.parseScanned("https://go-link.org/g/$invite"))
        assertEquals(InviteTarget.Link(invite), Invites.parseAppLink("https", "go-link.org", "/g/$invite", null))
        // Query parameters are still refused, key or not.
        assertNull(Invites.parseScanned("https://go-link.org/g/$invite?x=1#k=$key"))
        // The key never shows in the display text or in toString.
        val link = InviteTarget.Link(invite, key)
        assertTrue(link.group)
        assertEquals("https://go-link.org/g/$invite", link.display())
        assertFalse(link.toString().contains(key))
        assertEquals(key, Invites.groupKeyOf("#k=$key"))
    }

    @Test
    fun pins() {
        assertEquals(true, Invites.isPin("012345"))
        assertEquals(false, Invites.isPin("12345"))
        assertEquals(false, Invites.isPin("12345a"))
        assertEquals("123 456 789", InviteTarget.Code("123456789").display())
    }
}
