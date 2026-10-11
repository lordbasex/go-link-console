// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
package org.golink.player.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class RoomStateTest {
    @Test
    fun parsesARoomState() {
        val m = parseObject(
            """{"type":"room_state","max_players":2,"voice":true,"chat":false,
            "info":{"title":"Friday","game":"Some Game","host":"Fede","art":"QUJD"},
            "seats":[{"name":"Ana","local_player":0,"you":true},null,{"name":"extra"}],
            "queue":[{"position":1,"name":"Guest 9F3A","you":false}],
            "spectators":[{"name":"Bo","you":false}],
            "you":{"name":"Ana","ports":[1],"queue_positions":[],"spectator":false,
              "swap_offers":[{"from":2,"to":1,"name":"Bo"},{"from":1,"to":1},{"from":3,"to":1}],"swap_asked":[]},
            "pausable":true,"paused":true,"paused_by":"Ana",
            "controls":{"players":2,"buttons":9,"control":"joy4way"},"recording":true,
            "picture":{"style":"crt","bands":"frame"}}""",
        )!!
        val s = RoomMessages.parseRoomState(m)!!
        assertEquals(2, s.maxPlayers)
        assertFalse(s.chat)
        assertEquals(listOf(SeatView(1, "Ana", 0, true), null), s.seats)
        assertEquals(RoomInfoView("Friday", "Some Game", "Fede", "QUJD"), s.info)
        assertEquals(listOf(SwapView(2, 1, "Bo")), s.you.swapOffers)
        assertEquals(GameControls(2, 6, "joy4way"), s.controls)
        assertEquals(Me.Player(listOf(1)), s.me)
        assertTrue(s.paused && s.pausable && s.recording)
        assertEquals(Picture(PictureStyle.CRT, PictureBands.FRAME), s.picture)
        assertEquals("9F3A", RoomMessages.guestId(s.queue[0].name))
        assertNull(RoomMessages.guestId("Ana"))
    }

    @Test
    fun defaultsForOlderDevices() {
        val s = RoomMessages.parseRoomState(parseObject("""{"type":"room_state","you":{"queue_positions":[3,2]}}""")!!)!!
        assertEquals(1, s.maxPlayers)
        assertTrue(s.voice)
        assertTrue(s.chat)
        assertEquals(GameControls.DEFAULT, s.controls)
        assertEquals(Me.Queue(2), s.me)
        assertNull(s.picture)
        // An unknown value means no room default at all.
        val odd = RoomMessages.parseRoomState(parseObject("""{"type":"room_state","picture":{"style":"vhs","bands":"black"}}""")!!)!!
        assertNull(odd.picture)
        assertNull(RoomMessages.parseRoomState(parseObject("""{"type":"chat"}""")!!))
    }

    @Test
    fun chatLines() {
        val user = RoomMessages.parseChat(parseObject("""{"type":"chat","name":"Ana","port":1,"role":"player","text":"hi","ts":5}""")!!)
        assertEquals(ChatLine.User("Ana", 1, "player", "hi", 5.0), user)
        val sys = RoomMessages.parseChat(
            parseObject("""{"type":"chat","system":"Ana took seat P2","event":"took_seat","args":{"name":"Ana","port":2},"ts":1}""")!!,
        )
        assertEquals(ChatLine.System("Ana took seat P2", 1.0, ChatEvent.TOOK_SEAT, ChatArgs("Ana", 2, "", 0)), sys)
        val unknown = RoomMessages.parseChat(parseObject("""{"type":"chat","system":"new thing","event":"future_event"}""")!!)
        assertEquals(ChatLine.System("new thing", 0.0), unknown)
        assertNull(RoomMessages.parseChat(parseObject("""{"type":"chat","text":"no name"}""")!!))
    }

    @Test
    fun typingAndStats() {
        val who = RoomMessages.parseTyping(parseObject("""{"type":"typing","names":[{"name":"Bo","port":2},{"name":""}]}""")!!)
        assertEquals(listOf(TypingView("Bo", 2)), who)
        val st = RoomMessages.parseStreamStats(parseObject("""{"type":"stream_stats","fps":59.9,"aspect":1.3333}""")!!)
        assertEquals(StreamStatsView(59.9, 1.3333), st)
        assertEquals(StreamStatsView(null, null), RoomMessages.parseStreamStats(parseObject("""{"type":"stream_stats","aspect":9}""")!!))
    }

    @Test
    fun pauseRequestFields() {
        val s = RoomMessages.parseRoomState(
            parseObject("""{"type":"room_state","host_online":false,"you":{"pause_asked":{"expires_at":"2026-09-29T10:00:30Z"}}}""")!!,
        )!!
        assertFalse(s.hostOnline)
        assertEquals(PauseAsk("2026-09-29T10:00:30Z"), s.you.pauseAsked)
        val none = RoomMessages.parseRoomState(parseObject("""{"type":"room_state","host_online":true,"you":{"pause_asked":null}}""")!!)!!
        assertTrue(none.hostOnline)
        assertNull(none.you.pauseAsked)
        // Older devices send neither: the host counts as online.
        assertTrue(RoomMessages.parseRoomState(parseObject("""{"type":"room_state"}""")!!)!!.hostOnline)
        val declined = RoomMessages.parseChat(parseObject("""{"type":"chat","system":"The host declined","event":"pause_declined","ts":3}""")!!)
        assertEquals(ChatLine.System("The host declined", 3.0, ChatEvent.PAUSE_DECLINED), declined)
    }

    @Test
    fun handsVoiceSlotsAndSpectators() {
        val s = RoomMessages.parseRoomState(
            parseObject(
                """{"type":"room_state","max_players":2,
                "seats":[{"name":"Ana","peer":"p-ana"},{"name":"Bo"}],
                "queue":[{"position":1,"name":"Cy","you":false,"peer":"p-cy"}],
                "spectators":[{"name":"Di","you":true,"hand":true},{"name":"Ed","hand":"yes"}],
                "you":{"name":"Di","spectator":true,"hand":true,"hand_again":"2026-10-11T10:00:30Z","held":true},
                "spectators_hear":true,"spectator_saver":true,
                "voice_slots":[{"name":"Bo","port":2,"you":false},null,{"name":"Cy","port":0},{"name":"Zed","port":7,"you":true},{"name":"extra","port":1}]}""",
            )!!,
        )!!
        assertEquals(SeatView(1, "Ana", 0, false, "p-ana"), s.seats[0])
        assertEquals(QueueEntry(1, "Cy", false, "p-cy"), s.queue[0])
        assertEquals(listOf(Spectator("Di", true, "", true), Spectator("Ed", false, "", false)), s.spectators)
        assertTrue(s.you.hand && s.you.held)
        assertEquals(java.time.Instant.parse("2026-10-11T10:00:30Z").toEpochMilli(), s.you.handAgain)
        assertTrue(s.spectatorsHear && s.spectatorSaver)
        // Four slots; a port outside the room's seats counts as the queue (0).
        assertEquals(listOf(VoiceSlot("Bo", 2, false), null, VoiceSlot("Cy", 0, false), VoiceSlot("Zed", 0, true)), s.voiceSlots)
        assertEquals(Me.Spectator, s.me)
        assertFalse(s.canTalk)

        // Older devices send none of it.
        val old = RoomMessages.parseRoomState(parseObject("""{"type":"room_state","you":{"queue_positions":[1],"hand_again":"soon"}}""")!!)!!
        assertEquals(List<VoiceSlot?>(4) { null }, old.voiceSlots)
        assertFalse(old.spectatorsHear || old.spectatorSaver || old.you.hand || old.you.held)
        assertEquals(0L, old.you.handAgain)
        assertTrue(old.canTalk) // the queue talks too
    }

    @Test
    fun silencingFollowsThePersonAcrossSlots() {
        val slots = listOf(VoiceSlot("Bo", 2, false), null, VoiceSlot("Cy", 0, false), VoiceSlot("Ana", 1, false))
        assertEquals(setOf(1, 3), VoiceSlots.silencedSlots(slots, setOf("Bo", "Cy")))
        // Bo moves to another slot: the silence moves with him.
        val later = listOf(VoiceSlot("Cy", 0, false), null, null, VoiceSlot("Bo", 2, false))
        assertEquals(setOf(1, 4), VoiceSlots.silencedSlots(later, setOf("Bo", "Cy")))
        assertEquals(emptySet<Int>(), VoiceSlots.silencedSlots(later, emptySet()))
    }

    @Test
    fun handAndHostChatEventsAndRefusals() {
        val events = mapOf(
            "let_queue" to ChatEvent.LET_QUEUE,
            "play_now" to ChatEvent.PLAY_NOW,
            "first_in_queue" to ChatEvent.FIRST_IN_QUEUE,
            "removed" to ChatEvent.REMOVED,
            "hand_later" to ChatEvent.HAND_LATER,
            "sent_to_queue" to ChatEvent.SENT_TO_QUEUE,
            "sent_to_watch" to ChatEvent.SENT_TO_WATCH,
        )
        for ((wire, event) in events) {
            val line = RoomMessages.parseChat(parseObject("""{"type":"chat","system":"x","event":"$wire","args":{"name":"Host","name2":"Di","port":2},"ts":1}""")!!)
            assertEquals(ChatLine.System("x", 1.0, event, ChatArgs("Host", 2, "Di", 0)), line)
        }
        assertEquals("queue_ask_host", RoomMessages.parseRefusal(parseObject("""{"type":"error","code":"queue_ask_host","error":"ask"}""")!!))
        assertNull(RoomMessages.parseRefusal(parseObject("""{"type":"error","error":"no code"}""")!!))
        assertNull(RoomMessages.parseRefusal(parseObject("""{"type":"chat","code":"x"}""")!!))
    }
}
