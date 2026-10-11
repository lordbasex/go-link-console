// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  APP,
  TRASH_DAYS,
  freeSeats,
  parseRoomMeta,
  type ManagedRoom,
  type RoomMeta,
} from "@go-link/shared";
import { GuestJoinPage } from "./GuestJoinPage";
import { t } from "../i18n";
import { LOBBY_REFRESH_MS } from "../config";
import { DEMO_ROOMS } from "../fixtures";
import { useSignal } from "../signal/SignalProvider";
import { GamepadIcon, PlusIcon } from "../components/Icons";
import { Chip, HeroTile, PageHero } from "../components/ui/PageHero";
import { SeatChips } from "../components/Seats";
import {
  RoomActions,
  STATE_ORDER,
  StarIcon,
  daysLeft,
  firstLine,
  matchesRoom,
  useRoomControls,
} from "../components/device/RoomControls";
import { useThumbKind, useThumbnail } from "../components/device/useThumbnail";
import {
  Skeleton,
  SkeletonCards,
  SkeletonRows,
} from "../components/ui/Skeleton";
import { GameArt } from "../components/ui/GameArt";

type Filter = "all" | "live" | "paused" | "favorites" | "archived" | "trash";
// Favorites, archived and trash only exist for the host's own rooms.
const PUBLIC_FILTERS: Filter[] = ["all", "live", "paused"];
const ALL_FILTERS: Filter[] = [
  "all",
  "live",
  "paused",
  "favorites",
  "archived",
  "trash",
];

interface LobbyRoom {
  roomId: string;
  meta: RoomMeta;
}

type Load =
  | { kind: "loading" }
  | { kind: "ready"; rooms: LobbyRoom[] }
  | { kind: "offline" };

/** Public rooms from signalhub, refreshed periodically. */
function usePublicRooms(): Load {
  const { client, state, demo } = useSignal();
  const [load, setLoad] = useState<Load>(
    demo ? { kind: "ready", rooms: DEMO_ROOMS } : { kind: "loading" },
  );

  useEffect(() => {
    if (demo) return;
    if (state !== "open") {
      setLoad((cur) =>
        cur.kind === "ready"
          ? cur
          : { kind: state === "closed" ? "offline" : "loading" },
      );
      return;
    }
    let alive = true;
    const fetchRooms = async () => {
      try {
        const reply = await client.request({ type: "rooms_list", app: APP }, [
          "rooms",
        ]);
        const rooms: LobbyRoom[] = [];
        for (const r of reply.rooms ?? []) {
          const meta = parseRoomMeta(r.meta);
          if (meta) rooms.push({ roomId: r.room_id, meta });
        }
        if (alive) setLoad({ kind: "ready", rooms });
      } catch {
        if (alive)
          setLoad((cur) => (cur.kind === "ready" ? cur : { kind: "offline" }));
      }
    };
    void fetchRooms();
    const timer = setInterval(fetchRooms, LOBBY_REFRESH_MS);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [client, state, demo]);

  // A lost connection keeps showing the last list, marked offline below.
  return load;
}

/** A public room of anyone, by filter and search. */
function publicMatches(room: LobbyRoom, filter: Filter, q: string): boolean {
  if (
    q &&
    !`${room.meta.game} ${room.meta.title} ${room.meta.host}`
      .toLowerCase()
      .includes(q)
  )
    return false;
  switch (filter) {
    case "all":
      return true;
    case "live":
      return !room.meta.paused;
    case "paused":
      return room.meta.paused;
    default:
      return false; // favorites, archived and trash are the host's own
  }
}

/** One of the host's own rooms, by filter and search. */
function ownMatches(room: ManagedRoom, filter: Filter, q: string): boolean {
  if (q && !`${room.game} ${room.name}`.toLowerCase().includes(q)) return false;
  return matchesRoom(room, filter);
}

// A placeholder color per game (when the host has no thumbnail), from the
// design tokens, always the same for the same game.
const ART_COLORS = [
  "--color-voice",
  "--color-p4",
  "--color-accent",
  "--color-p3",
  "--color-accent-dim",
];
function artColor(game: string): string {
  let h = 0;
  for (const c of game) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return `var(${ART_COLORS[h % ART_COLORS.length]})`;
}

const VIEW_KEY = "go-link.rooms-view";
type View = "cards" | "list";
export function readRoomsView(): View {
  try {
    return window.localStorage.getItem(VIEW_KEY) === "list" ? "list" : "cards";
  } catch {
    return "cards";
  }
}
const initials = (s: string) =>
  s
    .replace(/[^A-Za-z0-9 ]/g, "")
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();

function RoomCard({ room, list = false }: { room: LobbyRoom; list?: boolean }) {
  const { meta } = room;
  const free = freeSeats(meta);
  const full = free === 0;
  const badge = meta.paused
    ? t.lobby.badgePaused
    : full
      ? meta.queue > 0
        ? t.lobby.badgeFullQueue(meta.queue)
        : t.lobby.badgeFull
      : t.lobby.badgeFree(free);
  const pill = (
    <span
      className={`lcard-pill${meta.paused ? " is-paused" : full ? " is-full" : ""}`}
    >
      <i />
      {badge}
    </span>
  );
  const cta = (
    <Link
      to={`/r/${room.roomId}`}
      className={`button ${full || meta.paused ? "button-secondary" : "button-primary"} button-compact`}
    >
      {meta.paused ? t.lobby.watchPaused : full ? t.lobby.queue : t.lobby.play}
    </Link>
  );
  if (list) {
    return (
      <tr
        className={`lrow${meta.paused ? " is-paused" : ""}`}
        style={{ ["--art-color" as string]: artColor(meta.game) }}
      >
        <td>
          <div className="lrow-room">
            <span className={`lrow-art${meta.art ? " has-art" : ""}`}>
              {meta.art ? <img src={meta.art} alt="" /> : initials(meta.game)}
            </span>
            <div className="lrow-main">
              <span className="lrow-title" title={meta.title}>
                <span className="lrow-name">{meta.title}</span>
              </span>
              <span className="small muted lcard-line">{meta.game}</span>
            </div>
          </div>
        </td>
        <td>{pill}</td>
        <td className="lrow-col-seats">
          <SeatChips players={meta.players} max={meta.maxPlayers} />
        </td>
        <td className="lrow-col-activity small faint">
          {t.lobby.watching(meta.spectators)}
        </td>
        <td className="lrow-col-host small muted">
          {meta.host ? t.lobby.host(meta.host) : "–"}
        </td>
        <td className="lrow-end">{cta}</td>
      </tr>
    );
  }
  return (
    <article
      className={`lcard${meta.paused ? " is-paused" : ""}`}
      style={{ ["--art-color" as string]: artColor(meta.game) }}
    >
      <div className={`lcard-art${meta.art ? " has-art" : ""}`}>
        {meta.art ? (
          <img src={meta.art} alt="" />
        ) : (
          <span>{meta.game.toUpperCase()}</span>
        )}
      </div>
      <div className="lcard-body">
        <span
          className={`lcard-pill${meta.paused ? " is-paused" : full ? " is-full" : ""}`}
        >
          <i />
          {badge}
        </span>
        <h2 className="lcard-title">{meta.title}</h2>
        <span className="small muted lcard-line">{meta.game}</span>
        <span className="small faint lcard-line">
          {[
            meta.host && t.lobby.host(meta.host),
            t.lobby.watching(meta.spectators),
          ]
            .filter(Boolean)
            .join(" · ")}
        </span>
        <SeatChips players={meta.players} max={meta.maxPlayers} />
        <Link
          to={`/r/${room.roomId}`}
          className={
            full || meta.paused
              ? "button button-secondary button-block lcard-cta"
              : "button button-primary button-block lcard-cta"
          }
        >
          {meta.paused
            ? t.lobby.watchPaused
            : full
              ? t.lobby.queue
              : t.lobby.play}
        </Link>
      </div>
    </article>
  );
}

/** One of the host's own rooms: its state, favorite and every action. */
function OwnRoomCard({
  room,
  art: lobbyArt,
  thumb,
  onAction,
  onStart,
  list = false,
}: {
  room: ManagedRoom;
  art: string | null;
  thumb: boolean;
  onAction: (action: string) => void;
  onStart: () => void;
  list?: boolean;
}) {
  const own = useThumbnail(
    room.rom,
    useThumbKind(),
    list ? "mini" : "card",
    thumb,
  );
  const art = own ?? lobbyArt;
  const off = room.state === "archived" || room.state === "trash";
  const detail =
    room.state === "live"
      ? t.gameRooms.liveDetail(room.players, room.spectators)
      : room.state === "paused"
        ? t.gameRooms.pausedDetail
        : room.state === "trash"
          ? t.gameRooms.trashDetail(daysLeft(room))
          : room.autosave
            ? t.gameRooms.archivedDetail
            : t.gameRooms.archivedOff;
  if (list) {
    return (
      <tr
        className={`lrow is-own is-${room.state}${room.favorite ? " is-favorite" : ""}`}
        style={{ ["--art-color" as string]: artColor(room.game) }}
      >
        <td>
          <div className="lrow-room">
            <span
              className={`lrow-art${art ? " has-art" : ""}${off ? " is-off" : ""}`}
            >
              {art ? <GameArt src={art} /> : initials(room.game)}
            </span>
            <div className="lrow-main">
              <span className="lrow-title" title={room.name}>
                {room.favorite && <StarIcon filled />}
                <span className="lrow-name">{room.name}</span>
              </span>
              <span className="small muted lcard-line">{room.game}</span>
            </div>
          </div>
        </td>
        <td>
          <span className={`groom-pill is-${room.state}`}>
            <i />
            {t.gameRooms.state[room.state]}
          </span>
        </td>
        <td className="lrow-col-seats">
          {off ? (
            <span className="faint">–</span>
          ) : (
            <SeatChips players={room.players} max={room.maxPlayers} />
          )}
        </td>
        <td
          className="lrow-col-activity small faint"
          title={room.lastError || undefined}
        >
          {room.lastError && room.state === "archived" ? (
            <span className="groom-error">
              {t.gameRooms.failed(firstLine(room.lastError))}
            </span>
          ) : (
            detail
          )}
          {room.saves.length > 0 && (
            <span className="lrow-sub">
              {t.gameRooms.saves(room.saves.length)}
            </span>
          )}
        </td>
        <td className="lrow-col-host small muted">
          {room.public ? t.lobby.yourRoom : t.lobby.yourPrivateRoom}
        </td>
        <td className="lrow-end">
          <RoomActions room={room} onAction={onAction} onStart={onStart} />
        </td>
      </tr>
    );
  }
  return (
    <article
      className={`lcard is-own is-${room.state}${room.favorite ? " is-favorite" : ""}`}
      style={{ ["--art-color" as string]: artColor(room.game) }}
    >
      <div
        className={`lcard-art${art ? " has-art" : ""}${off ? " is-off" : ""}`}
      >
        {art ? (
          <GameArt src={art} />
        ) : (
          <span>{room.game.toUpperCase()}</span>
        )}
      </div>
      <div className="lcard-body">
        <div className="lcard-top">
          <span className={`groom-pill is-${room.state}`}>
            <i />
            {t.gameRooms.state[room.state]}
          </span>
          <span className="lcard-mine">
            {room.public ? t.lobby.yourRoom : t.lobby.yourPrivateRoom}
          </span>
          <button
            type="button"
            className={`groom-star${room.favorite ? " is-on" : ""}`}
            aria-pressed={room.favorite}
            aria-label={
              room.favorite ? t.gameRooms.unfavorite : t.gameRooms.favorite
            }
            title={
              room.favorite ? t.gameRooms.unfavorite : t.gameRooms.favorite
            }
            onClick={() => onAction(room.favorite ? "unfavorite" : "favorite")}
          >
            <StarIcon filled={room.favorite} />
          </button>
        </div>
        <h2 className="lcard-title">
          {room.name}
        </h2>
        <span className="small muted lcard-line">{room.game}</span>
        <span className="small faint lcard-line">
          {detail}
          {room.saves.length > 0 &&
            ` · ${t.gameRooms.saves(room.saves.length)}`}
        </span>
        {room.lastError && room.state === "archived" && (
          <span className="small groom-error lcard-line" title={room.lastError}>
            {t.gameRooms.failed(firstLine(room.lastError))}
          </span>
        )}
        {!off && <SeatChips players={room.players} max={room.maxPlayers} />}
        <RoomActions room={room} onAction={onAction} onStart={onStart} />
      </div>
    </article>
  );
}

export function LobbyPage() {
  const { demo, hostLink, savedLink } = useSignal();
  // A guest (no device of its own) gets the code and PIN form: games are
  // private, so there is no list to show.
  if (!demo && !hostLink && !savedLink) return <GuestJoinPage />;
  return <OwnerLobby />;
}

function OwnerLobby() {
  const load = usePublicRooms();
  const { state, demo, hostLink, linkedDevice, savedLink, deviceOffline } =
    useSignal();
  const [filter, setFilter] = useState<Filter>("all");
  const [query, setQuery] = useState("");
  const [view, setView] = useState<View>(readRoomsView);
  const pickView = (v: View) => {
    setView(v);
    try {
      window.localStorage.setItem(VIEW_KEY, v);
    } catch {
      // storage blocked: the choice lasts for this page
    }
  };
  const publicRooms = load.kind === "ready" ? load.rooms : [];
  // The host's own rooms come from the linked device; the lobby shows
  // them once, with their controls, even when they are public too.
  const mine = useMemo(
    () => (hostLink ? (linkedDevice.status?.rooms ?? []) : []),
    [hostLink, linkedDevice.status?.rooms],
  );
  const controls = useRoomControls(mine);
  // The device's rooms are on their way (linking, or a remembered device
  // reconnecting): placeholders, never an empty list that then fills up.
  const waiting = demo
    ? load.kind === "loading"
    : hostLink
      ? !linkedDevice.status && linkedDevice.state !== "failed"
      : savedLink !== null && !deviceOffline;
  const mineIds = new Set(mine.map((r) => r.roomId).filter(Boolean));
  // Games are private now: the public directory only matters in demo mode.
  const others = demo ? publicRooms.filter((r) => !mineIds.has(r.roomId)) : [];
  const artOf = new Map(publicRooms.map((r) => [r.roomId, r.meta.art]));
  const q = query.trim().toLowerCase();
  const filters = mine.length > 0 ? ALL_FILTERS : PUBLIC_FILTERS;
  const count = (f: Filter) =>
    mine.filter((r) => matchesRoom(r, f)).length +
    others.filter((r) => publicMatches(r, f, "")).length;
  const ownVisible = mine
    .filter((r) => ownMatches(r, filter, q))
    .sort(
      (a, b) =>
        Number(b.favorite) - Number(a.favorite) ||
        STATE_ORDER.indexOf(a.state) - STATE_ORDER.indexOf(b.state) ||
        b.since.localeCompare(a.since),
    );
  const othersVisible = others.filter((r) => publicMatches(r, filter, q));
  const total = ownVisible.length + othersVisible.length;
  const playingNow =
    mine.reduce(
      (n, r) =>
        n + (r.state === "live" || r.state === "paused" ? r.players : 0),
      0,
    ) + others.reduce((n, r) => n + r.meta.players, 0);

  return (
    <div className="page">
      <div className="page-body lobby-page">
        <div className="stack-lg lobby-main">
          <PageHero
            tile={
              <HeroTile status={state === "open" ? "live" : "idle"}>
                <GamepadIcon size={34} />
              </HeroTile>
            }
            eyebrow={t.lobby.eyebrow}
            title={t.lobby.title}
            subtitle={t.lobby.subtitle}
            chips={
              waiting ? (
                <>
                  <Skeleton width={120} height={28} round />
                  <Skeleton width={120} height={28} round />
                </>
              ) : (
                <>
                  <Chip tone="live" dot>
                    {t.lobby.liveChip(count("live"))}
                  </Chip>
                  <Chip>{t.lobby.playingChip(playingNow)}</Chip>
                </>
              )
            }
          />

          <div className="roms-toolbar">
            <label className="dash-search roms-search">
              <span className="visually-hidden">{t.lobby.searchLabel}</span>
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                aria-hidden="true"
              >
                <circle cx="11" cy="11" r="7" />
                <path d="m20 20-3.5-3.5" />
              </svg>
              <input
                id="search"
                type="search"
                placeholder={t.lobby.searchPlaceholder}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
              />
            </label>
            <div className="roms-toolbar-end">
              <span className="small faint">
                {t.roms.showing(total, mine.length + others.length)}
              </span>
              <div className="roms-view" role="group" aria-label={t.roms.view}>
                <button
                  type="button"
                  aria-label={t.roms.cards}
                  title={t.roms.cards}
                  aria-pressed={view === "cards"}
                  className={view === "cards" ? "is-on" : ""}
                  onClick={() => pickView("cards")}
                >
                  <svg
                    width="18"
                    height="18"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    aria-hidden="true"
                  >
                    <rect x="3" y="3" width="7" height="7" rx="1.5" />
                    <rect x="14" y="3" width="7" height="7" rx="1.5" />
                    <rect x="3" y="14" width="7" height="7" rx="1.5" />
                    <rect x="14" y="14" width="7" height="7" rx="1.5" />
                  </svg>
                </button>
                <button
                  type="button"
                  aria-label={t.roms.list}
                  title={t.roms.list}
                  aria-pressed={view === "list"}
                  className={view === "list" ? "is-on" : ""}
                  onClick={() => pickView("list")}
                >
                  <svg
                    width="18"
                    height="18"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    aria-hidden="true"
                  >
                    <path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01" />
                  </svg>
                </button>
              </div>
            </div>
          </div>

          <div className="lobby-filter-row">
            <div
              role="group"
              aria-label={t.lobby.filtersLabel}
              className="grooms-filters"
            >
              {filters.map((f) => (
                <button
                  key={f}
                  type="button"
                  className={f === filter ? "is-on" : ""}
                  aria-pressed={f === filter}
                  onClick={() => setFilter(f)}
                >
                  {t.gameRooms.filters[f]}
                  <span>{count(f)}</span>
                </button>
              ))}
            </div>
            <Link
              to="/create"
              className="icon-button icon-button-primary"
              aria-label={t.lobby.createRoom}
              data-tip={t.lobby.createRoom}
            >
              <PlusIcon />
            </Link>
          </div>

          {filter === "trash" && (
            <p className="grooms-note">{t.gameRooms.trashNote(TRASH_DAYS)}</p>
          )}
          {!demo && state !== "open" && load.kind !== "loading" && (
            <p className="notice" role="status">
              {t.lobby.offline}
            </p>
          )}
          {waiting &&
            (view === "list" ? (
              <SkeletonRows rows={4} label={t.lobby.loading} />
            ) : (
              <SkeletonCards cards={3} label={t.lobby.loading} />
            ))}
          {!waiting && publicRooms.length === 0 && mine.length === 0 && (
            <div className="empty-state">
              <p className="strong">
                {demo ? t.lobby.empty : t.lobby.emptyOwn}
              </p>
              <p className="muted">
                {demo ? t.lobby.emptyHint : t.lobby.emptyOwnHint}
              </p>
            </div>
          )}
          {(publicRooms.length > 0 || mine.length > 0) && total === 0 && (
            <p className="muted">{t.lobby.noMatches}</p>
          )}
          {view === "list" ? (
            total > 0 && (
              <div className="lobby-table-wrap">
                <table className="lobby-table">
                  <thead>
                    <tr>
                      <th scope="col">{t.lobby.cols.room}</th>
                      <th scope="col">{t.lobby.cols.state}</th>
                      <th scope="col" className="lrow-col-seats">
                        {t.lobby.cols.players}
                      </th>
                      <th scope="col" className="lrow-col-activity">
                        {t.lobby.cols.activity}
                      </th>
                      <th scope="col" className="lrow-col-host">
                        {t.lobby.cols.host}
                      </th>
                      <th scope="col" className="lrow-end">
                        {t.lobby.cols.actions}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {ownVisible.map((r) => (
                      <OwnRoomCard
                        list
                        key={r.id}
                        room={r}
                        art={artOf.get(r.roomId) ?? null}
                        thumb={controls.hasThumb(r.rom)}
                        onAction={(action) => controls.act(r, action)}
                        onStart={() => controls.start(r)}
                      />
                    ))}
                    {othersVisible.map((room) => (
                      <RoomCard key={room.roomId} room={room} list />
                    ))}
                  </tbody>
                </table>
              </div>
            )
          ) : (
            <div className="lobby-grid">
              {ownVisible.map((r) => (
                <OwnRoomCard
                  key={r.id}
                  room={r}
                  art={artOf.get(r.roomId) ?? null}
                  thumb={controls.hasThumb(r.rom)}
                  onAction={(action) => controls.act(r, action)}
                  onStart={() => controls.start(r)}
                />
              ))}
              {othersVisible.map((room) => (
                <RoomCard key={room.roomId} room={room} />
              ))}
            </div>
          )}
          {total > 0 && <p className="small faint">{t.lobby.oneHost}</p>}
        </div>
      </div>
      {controls.overlays}
    </div>
  );
}
