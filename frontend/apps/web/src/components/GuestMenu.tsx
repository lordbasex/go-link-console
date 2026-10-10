// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState } from "react";
import { t } from "../i18n";
import type { SeatCard } from "../pages/roomModel";
import { CloseIcon, GamepadIcon, HandIcon, LogOutIcon, MoreIcon, UserIcon } from "./Icons";

/** What the host can do with someone who is not playing (the device checks it is the host). */
export interface HostTools {
  /** The seats now, to say who a "play now" replaces. */
  seats: (SeatCard | null)[];
  onPromote: (peer: string, mode: "queue" | "play", port?: number) => void;
  onLater: (peer: string) => void;
  onRemove: (peer: string) => void;
}

/** The seat choices of "play now": a free seat first, or whose turn it takes. */
export function seatChoices(seats: (SeatCard | null)[]): { port: number; label: string; note: string }[] {
  return seats.map((s, i) => ({
    port: i + 1,
    label: t.hand.playAt(i + 1),
    note: s ? t.hand.instead(s.name) : t.hand.seatFree,
  }));
}

/** Seats a person: at once on a free seat, else the host picks whose seat. */
export function SeatPicker({ tools, peer, onDone }: { tools: HostTools; peer: string; onDone: () => void }) {
  return (
    <div className="seat-picker" role="group" aria-label={t.hand.pickSeat}>
      <span className="capsule-menu-title">{t.hand.pickSeat}</span>
      {seatChoices(tools.seats).map((c) => (
        <button
          key={c.port}
          type="button"
          role="menuitem"
          className="groom-menu-item"
          onClick={() => {
            tools.onPromote(peer, "play", c.port);
            onDone();
          }}
        >
          <GamepadIcon size={15} />
          <span className="stack-xxs">
            <span>{c.label}</span>
            <span className="small muted">{c.note}</span>
          </span>
        </button>
      ))}
    </div>
  );
}

/** Plays at once when a seat is free; the picker is only for a full room. */
export function playNow(tools: HostTools, peer: string): boolean {
  const free = tools.seats.findIndex((s) => s === null);
  if (free < 0) return false;
  tools.onPromote(peer, "play", free + 1);
  return true;
}

/**
 * The host's "…" beside someone in the queue or watching: play now (a seat,
 * or whose seat), to the queue (someone watching), not now (a raised hand)
 * and take out of the room.
 */
export function GuestMenu({
  tools,
  peer,
  name,
  spectator,
  hand = false,
}: {
  tools: HostTools;
  peer: string;
  name: string;
  spectator: boolean;
  hand?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", close);
    window.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
    };
  }, [open]);
  useEffect(() => {
    if (!open) setPicking(false);
  }, [open]);
  const done = () => setOpen(false);
  return (
    <div className="guest-menu" ref={rootRef}>
      <button
        type="button"
        className="icon-button icon-button-small"
        aria-label={t.hand.actionsFor(name)}
        title={t.hand.actionsFor(name)}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <MoreIcon size={16} />
      </button>
      {open && (
        <div className="groom-menu" role="menu" aria-label={t.hand.actionsFor(name)}>
          {picking ? (
            <SeatPicker tools={tools} peer={peer} onDone={done} />
          ) : (
            <>
              <button
                type="button"
                role="menuitem"
                className="groom-menu-item"
                onClick={() => (playNow(tools, peer) ? done() : setPicking(true))}
              >
                <GamepadIcon size={15} />
                {t.hand.playNow}
              </button>
              {spectator && (
                <button
                  type="button"
                  role="menuitem"
                  className="groom-menu-item"
                  onClick={() => {
                    tools.onPromote(peer, "queue");
                    done();
                  }}
                >
                  <UserIcon size={15} />
                  {t.hand.toQueue}
                </button>
              )}
              {hand && (
                <button
                  type="button"
                  role="menuitem"
                  className="groom-menu-item"
                  onClick={() => {
                    tools.onLater(peer);
                    done();
                  }}
                >
                  <CloseIcon size={15} />
                  {t.hand.notNow}
                </button>
              )}
              <button
                type="button"
                role="menuitem"
                className="groom-menu-item is-danger"
                onClick={() => {
                  tools.onRemove(peer);
                  done();
                }}
              >
                <LogOutIcon size={15} />
                {t.hand.removeOne(name)}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * The host's raised hands, as a button at the end of the players capsule
 * ("✋ 2") that opens the list: to the queue, play now or not now for each.
 */
export function RaisedHands({
  hands,
  tools,
}: {
  hands: { peer: string; name: string }[];
  tools: HostTools;
}) {
  const [open, setOpen] = useState(false);
  const [picking, setPicking] = useState<string | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", close);
    window.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
    };
  }, [open]);
  useEffect(() => {
    if (hands.length === 0) setOpen(false);
    if (picking && !hands.some((h) => h.peer === picking)) setPicking(null);
  }, [hands, picking]);
  if (hands.length === 0) return null;
  const label = t.hand.hands(hands.length);
  const done = () => {
    setPicking(null);
    setOpen(false);
  };
  return (
    <div className="capsule-seat raised-hands" role="listitem" ref={rootRef}>
      <button
        type="button"
        className="capsule-avatar capsule-hands"
        aria-label={label}
        title={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <HandIcon size={16} />
        <span className="capsule-hands-count">{hands.length}</span>
      </button>
      {open && (
        <div className="capsule-menu hands-menu" role="menu" aria-label={label}>
          <span className="capsule-menu-title">{label}</span>
          {picking ? (
            <SeatPicker tools={tools} peer={picking} onDone={done} />
          ) : (
            hands.map((h) => (
              <div key={h.peer} className="hand-row" role="group" aria-label={t.hand.raised(h.name)}>
                <span className="hand-name">
                  <HandIcon size={14} />
                  {h.name}
                </span>
                <span className="hand-actions">
                  <button type="button" role="menuitem" className="button button-primary button-small" onClick={() => (playNow(tools, h.peer) ? done() : setPicking(h.peer))}>
                    {t.hand.playNow}
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    className="button button-secondary button-small"
                    onClick={() => {
                      tools.onPromote(h.peer, "queue");
                      done();
                    }}
                  >
                    {t.hand.toQueue}
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    className="button button-ghost button-small"
                    onClick={() => {
                      tools.onLater(h.peer);
                      done();
                    }}
                  >
                    {t.hand.notNow}
                  </button>
                </span>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}
