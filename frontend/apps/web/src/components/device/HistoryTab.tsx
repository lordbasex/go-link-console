// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { formatBytes, parseHistory, parseRecordings, shortRunId, type HistoryItem, type HistoryReason, type RecordingInfo } from "@go-link/shared";
import { getLang, t } from "../../i18n";
import { useRomsByName } from "./useLibrary";
import { useSignal } from "../../signal/SignalProvider";
import { ConfirmDialog } from "../RemapDialog";
import { DownloadIcon, PlayIcon, PulseIcon, TrashIcon } from "../Icons";
import { DownloadDialog, trackNames } from "../Recordings";
import { localName } from "../../pages/roomModel";
import { SkeletonRows } from "../ui/Skeleton";
import { useInfiniteList } from "../ui/useInfiniteList";
import { useThumbKind, useThumbnail } from "./useThumbnail";
import { GameArt } from "../ui/GameArt";

type Filter = "all" | HistoryReason;
const FILTERS: Filter[] = ["all", "archived", "deleted", "device_stopped", "failed"];
const PILL: Record<HistoryReason, string> = {
  archived: "is-archived",
  deleted: "is-trash",
  failed: "is-trash",
  device_stopped: "is-paused",
};

const initials = (s: string) =>
  s
    .replace(/[^A-Za-z0-9 ]/g, "")
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();

/** 3725 s -> "1 h 02 min"; 95 s -> "1 min"; under a minute -> "< 1 min". */
export function formatDuration(ms: number): string {
  const min = Math.floor(Math.max(0, ms) / 60000);
  if (min < 1) return "< 1 min";
  if (min < 60) return `${min} min`;
  return `${Math.floor(min / 60)} h ${String(min % 60).padStart(2, "0")} min`;
}

function formatWhen(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "–";
  return new Intl.DateTimeFormat(getLang(), { dateStyle: "medium", timeStyle: "short" }).format(d);
}

/** When a game ended: only the time when it ended the day it started. */
function formatEnd(startIso: string, endIso: string): string {
  const start = new Date(startIso);
  const end = new Date(endIso);
  if (Number.isNaN(end.getTime())) return "";
  const sameDay = start.toDateString() === end.toDateString();
  return new Intl.DateTimeFormat(getLang(), sameDay ? { timeStyle: "short" } : { dateStyle: "medium", timeStyle: "short" }).format(end);
}

/** The "History" tab of My device: every game the device ran, newest first. */
export function HistoryTab() {
  const { hostLink, sendToDevice, onDeviceMessage, linkedDevice } = useSignal();
  const [items, setItems] = useState<HistoryItem[] | null>(null);
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const [confirming, setConfirming] = useState(false);
  const [recBytes, setRecBytes] = useState<{ n: number; bytes: number } | null>(null);
  const [downloading, setDownloading] = useState<RecordingInfo | null>(null);
  const [deleting, setDeleting] = useState<{ kind: "rec"; rec: RecordingInfo } | { kind: "game"; item: HistoryItem } | null>(null);
  const library = linkedDevice.status?.library;
  // Ask again once the data link is up: a request sent before is dropped.
  const ready = linkedDevice.state === "connected" && linkedDevice.status !== null;

  useEffect(() => {
    const off = onDeviceMessage((msg) => {
      const list = parseHistory(msg);
      if (list) {
        setItems(list);
        return;
      }
      // A recording was deleted (or listed): refresh the totals and rows.
      const recs = parseRecordings(msg);
      if (recs) {
        setRecBytes({ n: recs.items.length, bytes: recs.bytes });
        sendToDevice({ type: "get_history" });
      }
    });
    sendToDevice({ type: "get_history" });
    sendToDevice({ type: "get_recordings" });
    return off;
  }, [hostLink, ready, sendToDevice, onDeviceMessage]);

  const all = useMemo(() => items ?? [], [items]);
  const visible = useMemo(() => {
    const q = query.trim().toLowerCase().replace(/^#/, "");
    return all.filter(
      (h) =>
        (filter === "all" || h.reason === filter) &&
        (!q ||
          `${h.id ?? ""} ${h.name} ${h.game} ${h.rom} ${h.people.map((p) => `${p.name} ${p.ip}`).join(" ")}`
            .toLowerCase()
            .includes(q)),
    );
  }, [all, filter, query]);
  const { shown, more, sentinel, loadMore } = useInfiniteList(visible, `${filter}|${query}`);
  const count = (f: Filter) => (f === "all" ? all.length : all.filter((h) => h.reason === f).length);
  const sets = useRomsByName(useMemo(() => shown.map((h) => h.rom), [shown]));
  const hasThumb = (rom: string) => sets.get(rom)?.thumbs[library?.thumbKind ?? "boxart"] ?? false;

  return (
    <section className="history" aria-label={t.history.title}>
      <div className="roms-toolbar">
        <label className="dash-search roms-search">
          <span className="visually-hidden">{t.dash.search}</span>
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true">
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.5-3.5" />
          </svg>
          <input type="search" placeholder={t.history.search} value={query} onChange={(e) => setQuery(e.target.value)} />
        </label>
        <div className="roms-toolbar-end">
          {recBytes && recBytes.n > 0 && (
            <span className="chip">{t.rec.total(recBytes.n, formatBytes(recBytes.bytes))}</span>
          )}
          {items && <span className="small faint">{t.roms.showing(visible.length, all.length)}</span>}
          <button
            type="button"
            className="icon-button icon-button-danger"
            aria-label={t.history.clear}
            data-tip={t.history.clear}
            disabled={!items || items.length === 0}
            onClick={() => setConfirming(true)}
          >
            <TrashIcon />
          </button>
        </div>
      </div>

      <div className="grooms-filters" role="group" aria-label={t.dash.filter}>
        {FILTERS.filter((f) => f === "all" || count(f) > 0).map((f) => (
          <button key={f} type="button" className={f === filter ? "is-on" : ""} aria-pressed={f === filter} onClick={() => setFilter(f)}>
            {f === "all" ? t.dash.filterAll : t.history.reason[f]}
            <span>{count(f)}</span>
          </button>
        ))}
      </div>

      {items === null ? (
        <SkeletonRows label={t.history.loading} />
      ) : all.length === 0 ? (
        <div className="empty-state">
          <p className="strong">{t.history.empty}</p>
          <p className="muted">{t.history.emptyHint}</p>
        </div>
      ) : visible.length === 0 ? (
        <p className="muted">{t.dash.noMatch}</p>
      ) : (
        <div className="lobby-table-wrap">
          <table className="lobby-table">
            <thead>
              <tr>
                <th scope="col">{t.history.cols.game}</th>
                <th scope="col" className="lrow-col-id">{t.history.cols.id}</th>
                <th scope="col">{t.history.cols.result}</th>
                <th scope="col" className="lrow-col-activity">{t.history.cols.started}</th>
                <th scope="col" className="lrow-col-seats">{t.history.cols.duration}</th>
                <th scope="col" className="lrow-col-host">{t.history.cols.players}</th>
                <th scope="col">{t.history.cols.recording}</th>
                <th scope="col" className="lrow-end">{t.lobby.cols.actions}</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((h) => (
                <HistoryRow
                  key={`${h.roomId}-${h.startedAt}`}
                  item={h}
                  thumb={hasThumb(h.rom)}
                  onDownload={setDownloading}
                  onDeleteRec={(rec) => setDeleting({ kind: "rec", rec })}
                  onDelete={() => setDeleting({ kind: "game", item: h })}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
      {more && (
        <div ref={sentinel} className="list-more">
          <button type="button" className="button button-secondary" onClick={loadMore}>
            {t.history.showMore}
          </button>
        </div>
      )}

      {downloading && <DownloadDialog rec={downloading} onClose={() => setDownloading(null)} />}
      {deleting && (
        <ConfirmDialog
          title={deleting.kind === "rec" ? t.rec.deleteTitle : t.history.deleteTitle}
          text={deleting.kind === "rec" ? t.rec.deleteText : t.history.deleteText}
          confirm={deleting.kind === "rec" ? t.rec.deleteOne : t.history.deleteOne}
          onCancel={() => setDeleting(null)}
          onConfirm={() => {
            if (deleting.kind === "rec") sendToDevice({ type: "delete_recording", id: deleting.rec.id });
            else sendToDevice({ type: "delete_history", id: deleting.item.id });
            setDeleting(null);
          }}
        />
      )}
      {confirming && (
        <ConfirmDialog
          title={t.history.clearTitle}
          text={t.history.clearText}
          confirm={t.history.clear}
          onCancel={() => setConfirming(false)}
          onConfirm={() => {
            setConfirming(false);
            sendToDevice({ type: "clear_history" });
          }}
        />
      )}
    </section>
  );
}

/** Who played at each port, with the address they came from, then how many watched. */
function HistoryPeople({ item }: { item: HistoryItem }) {
  // Games recorded before the device kept names only have the peaks.
  if (item.people.length === 0) return <span className="small muted">{t.history.peak(item.peakPlayers, item.peakSpectators)}</span>;
  const players = item.people.filter((p) => p.ports.length > 0);
  const watchers = item.people.length - players.length;
  return (
    <ul className="hist-people">
      {players.map((p, i) => (
        <li key={i}>
          {p.ports.map((port) => (
            <span key={port} className={`hist-port is-p${port}`}>{`P${port}`}</span>
          ))}
          <span className="hist-name">{localName(p.name)}</span>
          <span className="hist-ip mono" title={p.path === "relay" ? t.history.viaRelayHint : undefined}>
            {p.ip || (p.path === "relay" ? t.history.viaRelay : "–")}
          </span>
        </li>
      ))}
      {watchers > 0 && <li className="small faint">{t.history.watchers(watchers)}</li>}
    </ul>
  );
}

interface HistoryRowProps {
  item: HistoryItem;
  thumb: boolean;
  onDownload: (rec: RecordingInfo) => void;
  onDeleteRec: (rec: RecordingInfo) => void;
  onDelete: () => void;
}

function HistoryRow({ item, thumb, onDownload, onDeleteRec, onDelete }: HistoryRowProps) {
  const art = useThumbnail(item.rom, useThumbKind(), "mini", thumb);
  const ms = new Date(item.endedAt).getTime() - new Date(item.startedAt).getTime();
  return (
    <tr className="lrow">
      <td>
        <div className="lrow-room">
          <span className={`lrow-art${art ? " has-art" : ""}`}>{art ? <GameArt src={art} /> : initials(item.game)}</span>
          <div className="lrow-main">
            <span className="lrow-title" title={item.name}>
              <span className="lrow-name">{item.name || item.game}</span>
            </span>
            <span className="small muted lcard-line">{item.game}</span>
          </div>
        </div>
      </td>
      <td className="lrow-col-id">
        {item.id ? <GameId id={item.id} /> : <span className="small faint">–</span>}
      </td>
      <td>
        <div className="history-result">
          <span className={`groom-pill ${PILL[item.reason]}`}>
            <i />
            {t.history.reason[item.reason]}
          </span>
          <span className="small muted">{formatEnd(item.startedAt, item.endedAt)}</span>
        </div>
      </td>
      <td className="lrow-col-activity small muted">{formatWhen(item.startedAt)}</td>
      <td className="lrow-col-seats small mono">{Number.isFinite(ms) ? formatDuration(ms) : "–"}</td>
      <td className="lrow-col-host" title={t.history.peak(item.peakPlayers, item.peakSpectators)}>
        <HistoryPeople item={item} />
      </td>
      <td>
        {!item.recordings?.length ? (
          <span className="small faint">{t.rec.none}</span>
        ) : (
          <ul className="hist-recs">
            {item.recordings.map((rec) => (
              <li key={rec.id}>
                <span className="hist-rec-line">
                  <i className="rec-dot" aria-hidden="true" />
                  {`${formatDuration(rec.durationMs)} · ${formatBytes(rec.size)}${rec.tracks.includes("video") ? "" : ` (${t.rec.audioOnly})`}`}
                </span>
                <span className="small muted">{trackNames(rec.tracks)}</span>
                <span className="hist-rec-actions">
                  <button type="button" className="button button-primary button-compact" onClick={() => onDownload(rec)}>
                    <DownloadIcon size={16} />
                    {t.rec.download}
                  </button>
                  <button
                    type="button"
                    className="icon-button icon-button-danger"
                    aria-label={t.rec.deleteOne}
                    data-tip={t.rec.deleteOne}
                    onClick={() => onDeleteRec(rec)}
                  >
                    <TrashIcon />
                  </button>
                </span>
              </li>
            ))}
          </ul>
        )}
      </td>
      <td className="lrow-end">
        <button
          type="button"
          className="icon-button icon-button-danger"
          aria-label={t.history.deleteOne}
          data-tip={t.history.deleteOne}
          disabled={!item.id}
          onClick={onDelete}
        >
          <TrashIcon />
        </button>
        {item.id && (
          <Link
            to={`/device/history/${encodeURIComponent(item.roomId)}/network?run=${encodeURIComponent(item.id)}`}
            className="icon-button"
            aria-label={t.net.open(shortRunId(item.id))}
            data-tip={t.net.openShort}
          >
            <PulseIcon />
          </Link>
        )}
        <Link
          to={`/create?rom=${encodeURIComponent(item.rom)}`}
          className="icon-button"
          aria-label={t.history.playAgain(item.game)}
          data-tip={t.history.playAgainShort}
        >
          <PlayIcon />
        </Link>
      </td>
    </tr>
  );
}

/** A game's id (its first 8 characters); a click copies the whole id. */
export function GameId({ id }: { id: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className="hist-id mono"
      aria-label={t.history.copyId(shortRunId(id))}
      data-tip={copied ? t.history.idCopied : t.history.copyIdShort}
      onClick={() => {
        navigator.clipboard?.writeText(`#${id}`).then(
          () => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          },
          () => undefined,
        );
      }}
    >
      #{shortRunId(id)}
    </button>
  );
}

