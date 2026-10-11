// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef } from "react";
import type { Knock } from "@go-link/shared";
import { t } from "../i18n";
import { useSignal } from "../signal/SignalProvider";
import { playDing } from "./ding";

interface RoomKnocks {
  id: string;
  name: string;
  knocks: Knock[];
}

/**
 * The host's linked browser, on any page: people who came with a group
 * invitation and wait to be let in, per room, with Let in and Not now for
 * each and Let everyone in. A short sound plays when someone new arrives.
 */
export function KnockNotices() {
  const { linkedDevice, sendToDevice } = useSignal();
  const status = linkedDevice.status;
  const rooms: RoomKnocks[] = [];
  if (status?.room?.knocks?.length) rooms.push({ id: "test", name: status.room.title || t.dash.testPattern, knocks: status.room.knocks });
  for (const r of status?.rooms ?? []) if (r.knocks?.length) rooms.push({ id: r.id, name: r.name, knocks: r.knocks });

  // A sound for each person that was not waiting before.
  const seen = useRef(new Set<string>());
  const peers = rooms.flatMap((r) => r.knocks.map((k) => k.peer));
  const key = peers.join(",");
  useEffect(() => {
    const now = new Set(key ? key.split(",") : []);
    if ([...now].some((p) => !seen.current.has(p))) playDing();
    seen.current = now;
  }, [key]);

  if (rooms.length === 0) return null;
  const answer = (id: string, peer: string, accept: boolean) => sendToDevice({ type: "knock_answer", id, peer, accept });
  return (
    <section className="pause-ask-notices knock-notices" aria-label={t.knocks.title(peers.length)}>
      {rooms.map((r) => (
        <div key={r.id} className="pause-ask knock-notice" role="alertdialog" aria-labelledby={`knock-${r.id}`}>
          <div className="pause-ask-head">
            <span className="knock-hand" aria-hidden="true">
              ✋
            </span>
            <div className="stack-xxs">
              <span id={`knock-${r.id}`} className="pause-ask-title">
                {t.knocks.title(r.knocks.length)}
              </span>
              <span className="small muted">{t.knocks.in(r.name)}</span>
            </div>
          </div>
          <ul className="knock-list">
            {r.knocks.map((k) => {
              const who = k.name || t.knocks.noName;
              return (
                <li key={k.peer} className="knock-row">
                  <span className="knock-name">
                    {who}
                    {k.watch && <span className="knock-watch small muted"> · {t.knocks.watching}</span>}
                  </span>
                  <button type="button" className="button button-secondary button-small" aria-label={t.knocks.declineOne(who)} onClick={() => answer(r.id, k.peer, false)}>
                    {t.knocks.decline}
                  </button>
                  <button type="button" className="button button-primary button-small" aria-label={t.knocks.acceptOne(who)} onClick={() => answer(r.id, k.peer, true)}>
                    {t.knocks.accept}
                  </button>
                </li>
              );
            })}
          </ul>
          {r.knocks.length > 1 && (
            <button type="button" className="button button-primary button-block" onClick={() => r.knocks.forEach((k) => answer(r.id, k.peer, true))}>
              {t.knocks.acceptAll}
            </button>
          )}
        </div>
      ))}
    </section>
  );
}
