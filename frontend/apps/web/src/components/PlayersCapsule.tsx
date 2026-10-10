// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import type { SeatCard } from "../pages/roomModel";
import { RaisedHands, type HostTools } from "./GuestMenu";
import { EyeIcon, GamepadIcon, LogOutIcon, MicOffIcon, SoundOffIcon, SoundOnIcon, SwapIcon, UserIcon } from "./Icons";
import { portStyle } from "./Seats";

/** Swapping controllers with a seat: move there, or ask its player. */
export interface SeatSwap {
  onSwap: () => void;
  /** Your request is waiting for this player's answer. */
  waiting: boolean;
}

const initials = (name: string) =>
  name
    .replace(/\(\d\)$/, "")
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase() || "?";

/**
 * The players as round avatars in a floating capsule over the video, like a
 * video call: the seat's color as a ring, a glow while speaking and a badge
 * when muted. Tapping an avatar opens its actions (press that seat's
 * Start, swap controllers, move to a free seat, silence and, for the host,
 * free the seat: to the end of the queue or to watching, or take the player
 * out of the room). The host also gets the raised hands at the end.
 */
export function PlayersCapsule({
  seats,
  swapFor,
  onToggleSilence,
  onStart,
  onRelease,
  queueWaiting = false,
  hands = [],
  hostTools,
}: {
  seats: (SeatCard | null)[];
  swapFor: (port: number) => SeatSwap | undefined;
  onToggleSilence?: (port: number) => void;
  /** A seated player presses a seat's Start (any seat, like the arcade's panel). */
  onStart?: (port: number) => void;
  /** The host frees a seat. */
  onRelease?: (port: number, mode: "queue" | "watch") => void;
  /** Someone waits in the queue (sending to the queue frees the seat for them). */
  queueWaiting?: boolean;
  /** The host's view: spectators with a raised hand, oldest first. */
  hands?: { peer: string; name: string }[];
  /** The host's actions on people (raised hands, take out of the room). */
  hostTools?: HostTools;
}) {
  const [open, setOpen] = useState<number | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (open === null) return;
    const close = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(null);
    };
    const esc = (e: KeyboardEvent) => (e.key === "Escape" || e.code === "Escape") && setOpen(null);
    document.addEventListener("mousedown", close);
    window.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
    };
  }, [open]);

  return (
    <div className="players-capsule" ref={rootRef} role="list" aria-label={t.room.playersTitle}>
      {seats.map((seat, i) => {
        const port = i + 1;
        const swap = swapFor(port);
        const silence = seat && !seat.you && onToggleSilence ? () => onToggleSilence(port) : undefined;
        const label = seat
          ? `P${port} · ${seat.name}${seat.you ? ` (${t.room.you})` : ""} · ${seat.status}`
          : `P${port} · ${t.room.freeSeat}`;
        const start = seat && onStart ? () => onStart(port) : undefined;
        const release = seat && onRelease ? onRelease : undefined;
        const remove = seat && !seat.you && seat.peer && hostTools ? () => hostTools.onRemove(seat.peer!) : undefined;
        const hasMenu = !!swap || !!silence || !!start || !!release || !!remove;
        const avatar = (
          <>
            <span className="capsule-port">P{port}</span>
            <span className="capsule-initials">{seat ? initials(seat.name) : ""}</span>
            {seat?.tone === "muted" && (
              <span className="capsule-badge" aria-hidden="true">
                <MicOffIcon size={11} />
              </span>
            )}
            {seat?.silenced && (
              <span className="capsule-badge is-silenced" aria-hidden="true">
                <SoundOffIcon size={11} />
              </span>
            )}
          </>
        );
        const cls = `capsule-avatar${seat ? "" : " is-free"}${seat?.you ? " is-you" : ""}${seat?.tone === "speaking" ? " is-speaking" : ""}${swap?.waiting ? " is-waiting" : ""}`;
        return (
          <div key={port} className="capsule-seat" role="listitem" style={portStyle(port)}>
            {hasMenu ? (
              <button
                type="button"
                className={cls}
                aria-label={label}
                title={label}
                aria-haspopup="menu"
                aria-expanded={open === port}
                onClick={() => setOpen(open === port ? null : port)}
              >
                {avatar}
              </button>
            ) : (
              <span className={cls} role="img" aria-label={label} title={label}>
                {avatar}
              </span>
            )}
            {open === port && (
              <div className="capsule-menu" role="menu" aria-label={label}>
                <span className="capsule-menu-title">{label}</span>
                {start && (
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      start();
                      setOpen(null);
                    }}
                  >
                    <GamepadIcon size={15} />
                    {t.room.pressStart(port)}
                  </button>
                )}
                {swap && (
                  <button
                    type="button"
                    role="menuitem"
                    disabled={swap.waiting}
                    onClick={() => {
                      swap.onSwap();
                      setOpen(null);
                    }}
                  >
                    <SwapIcon size={15} />
                    {swap.waiting ? t.room.swapWaiting(seat?.name ?? "") : seat ? t.room.swapTo(port) : t.room.moveTo(port)}
                  </button>
                )}
                {silence && seat && (
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      silence();
                      setOpen(null);
                    }}
                  >
                    {seat.silenced ? <SoundOnIcon size={15} /> : <SoundOffIcon size={15} />}
                    {seat.silenced ? t.room.unsilence(seat.name) : t.room.silence(seat.name)}
                  </button>
                )}
                {release && seat && queueWaiting && (
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      release(port, "queue");
                      setOpen(null);
                    }}
                  >
                    <UserIcon size={15} />
                    {seat.you ? t.room.freeMySeat : t.room.sendToQueue(seat.name)}
                  </button>
                )}
                {release && seat && (
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      release(port, "watch");
                      setOpen(null);
                    }}
                  >
                    <EyeIcon size={15} />
                    {seat.you ? t.room.watchMyself : t.room.makeSpectator(seat.name)}
                  </button>
                )}
                {remove && seat && (
                  <button
                    type="button"
                    role="menuitem"
                    className="is-danger"
                    onClick={() => {
                      remove();
                      setOpen(null);
                    }}
                  >
                    <LogOutIcon size={15} />
                    {t.hand.removeOne(seat.name)}
                  </button>
                )}
              </div>
            )}
          </div>
        );
      })}
      {hostTools && <RaisedHands hands={hands} tools={hostTools} />}
    </div>
  );
}
