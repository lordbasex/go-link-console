// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import {
  TRASH_DAYS,
  type ManagedRoom,
  type ManagedRoomState,
} from "@go-link/shared";
import { t } from "../../i18n";
import { useRomsByName } from "./useLibrary";
import { useSignal } from "../../signal/SignalProvider";
import { ConfirmDialog } from "../RemapDialog";
import { RoomPictureDialog } from "../RoomPictureDialog";
import {
  ArchiveIcon,
  EnterIcon,
  MoreIcon,
  PauseIcon,
  PictureIcon,
  PlayIcon,
  PowerIcon,
  RestoreIcon,
  SaveIcon,
  TrashIcon,
} from "../Icons";
import { useThumbKind, useThumbnail } from "./useThumbnail";
import { GameArt } from "../ui/GameArt";

export type Filter =
  "all" | "live" | "paused" | "favorites" | "archived" | "trash";
type Start = { from: "continue" | "fresh" | "slot"; slot: number };

export const STATE_ORDER: ManagedRoomState[] = [
  "live",
  "paused",
  "archived",
  "trash",
];
const DAY = 24 * 60 * 60 * 1000;

export function matchesRoom(r: ManagedRoom, f: Filter): boolean {
  switch (f) {
    case "all":
      return r.state !== "trash";
    case "favorites":
      return r.favorite && r.state !== "trash";
    default:
      return r.state === f;
  }
}

/** The first line of an error, without the emulator's log that follows. */
export function firstLine(text: string): string {
  const line = text
    .split("\n")[0]!
    .replace(/; log:$/, "")
    .trim();
  return line.length > 140 ? `${line.slice(0, 140)}…` : line;
}

/** Days until a deleted room is removed for good. */
export function daysLeft(r: ManagedRoom): number {
  if (!r.deletedAt) return TRASH_DAYS;
  const end = new Date(r.deletedAt).getTime() + TRASH_DAYS * DAY;
  return Math.max(1, Math.ceil((end - Date.now()) / DAY));
}

/**
 * Controls for the host's own rooms: actions, the start dialog, the
 * delete-forever confirmation and a toast with each result.
 */
export function useRoomControls(rooms: ManagedRoom[]) {
  const { linkedDevice, hostLink, onDeviceMessage } = useSignal();
  const [toast, setToast] = useState("");
  const [starting, setStarting] = useState<ManagedRoom | null>(null);
  const [purging, setPurging] = useState<ManagedRoom | null>(null);
  const [archiving, setArchiving] = useState<ManagedRoom | null>(null);
  const [picturing, setPicturing] = useState<ManagedRoom | null>(null);
  const names = useRef(new Map<string, string>());
  names.current = new Map(rooms.map((r) => [r.id, r.name]));
  const toastTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const say = (text: string) => {
    setToast(text);
    clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(""), 3000);
  };
  useEffect(() => () => clearTimeout(toastTimer.current), []);
  useEffect(
    () =>
      onDeviceMessage((msg) => {
        const m = msg as {
          type?: string;
          id?: string;
          action?: string;
          ok?: boolean;
          error?: string;
          code?: string;
          limit?: number;
          slot?: number;
        };
        if (m.type === "room_created" && m.id && names.current.get(m.id)) {
          say(t.gameRooms.done.start(names.current.get(m.id)!));
        } else if (m.type === "room_error" && m.id) {
          say(deviceErrorText(m));
        } else if (m.type === "room_result" && m.id) {
          const name = names.current.get(m.id) ?? "";
          if (!m.ok) say(deviceErrorText(m));
          else if (m.action === "save")
            say(t.gameRooms.done.save(name, m.slot ?? 0));
          else if (
            m.action === "pause" ||
            m.action === "resume" ||
            m.action === "archive" ||
            m.action === "delete" ||
            m.action === "purge" ||
            m.action === "picture"
          )
            say(t.gameRooms.done[m.action](name));
        }
      }),
    [onDeviceMessage],
  );
  const send = (msg: unknown) => hostLink?.stream.sendControl(msg);
  const act = (r: ManagedRoom, action: string) => {
    if (action === "purge") setPurging(r);
    // The room's default picture for its guests: a dialog with the choices.
    else if (action === "picture") setPicturing(r);
    // A game that cannot be saved loses its place when archived: ask.
    else if (action === "archive" && r.noSaves) setArchiving(r);
    else send({ type: "room_action", id: r.id, action });
  };
  const library = linkedDevice.status?.library;
  const kind = library?.thumbKind ?? "boxart";
  const sets = useRomsByName(rooms.map((r) => r.rom));
  const hasThumb = (rom: string) => sets.get(rom)?.thumbs[kind] ?? false;
  const overlays = (
    <>
      {toast && (
        <div className="grooms-toast" role="status">
          {toast}
        </div>
      )}
      {starting && (
        <StartDialog
          room={starting}
          boxart={hasThumb(starting.rom)}
          onCancel={() => setStarting(null)}
          onStart={(st) => {
            send({
              type: "room_start",
              id: starting.id,
              from: st.from,
              slot: st.slot,
            });
            setStarting(null);
          }}
        />
      )}
      {archiving && (
        <ConfirmDialog
          title={t.gameRooms.archiveNoSavesTitle(archiving.name)}
          text={t.gameRooms.archiveNoSavesText}
          confirm={t.gameRooms.archiveAnyway}
          alternative={
            archiving.state === "live"
              ? {
                  label: t.gameRooms.pauseInstead,
                  onClick: () => {
                    send({ type: "room_action", id: archiving.id, action: "pause" });
                    setArchiving(null);
                  },
                }
              : undefined
          }
          onCancel={() => setArchiving(null)}
          onConfirm={() => {
            send({ type: "room_action", id: archiving.id, action: "archive" });
            setArchiving(null);
          }}
        />
      )}
      {picturing && (
        <RoomPictureDialog
          id={picturing.id}
          name={picturing.name}
          current={picturing.picture ?? null}
          send={send}
          onClose={() => setPicturing(null)}
        />
      )}
      {purging && (
        <ConfirmDialog
          title={t.gameRooms.purgeTitle}
          text={t.gameRooms.purgeText}
          confirm={t.gameRooms.purge}
          onCancel={() => setPurging(null)}
          onConfirm={() => {
            send({ type: "room_action", id: purging.id, action: "purge" });
            setPurging(null);
          }}
        />
      )}
    </>
  );
  return { act, start: setStarting, hasThumb, overlays };
}

interface ActionItem {
  id: string;
  label: string;
  icon: ReactNode;
  danger?: boolean;
  to?: string; // a link instead of an action
  run?: () => void;
}

/**
 * The words for a device error: known codes in the page's language, the
 * device's own text otherwise.
 */
export function deviceErrorText(m: { error?: string; code?: string; limit?: number }): string {
  if (m.code === "too_many_rooms" && typeof m.limit === "number" && m.limit > 0)
    return t.gameRooms.tooMany(m.limit);
  if (m.code === "no_saves") return t.gameRooms.noSaves;
  return t.gameRooms.error(m.error ?? "");
}

/** The room's actions: icon buttons (with a tooltip) and, in the ⋯ menu,
 * every action with its name. */
export function RoomActions({
  room,
  onAction,
  onStart,
}: {
  room: ManagedRoom;
  onAction: (action: string) => void;
  onStart: () => void;
}) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => (e.key === "Escape" || e.code === "Escape") && setOpen(false);
    document.addEventListener("mousedown", close);
    window.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
    };
  }, [open]);

  const act =
    (id: string): ActionItem["run"] =>
    () =>
      onAction(id);
  const items: ActionItem[] =
    room.state === "live"
      ? [
          ...(room.roomId
            ? [
                {
                  id: "open",
                  label: t.gameRooms.open,
                  icon: <EnterIcon />,
                  to: `/r/${room.roomId}`,
                },
              ]
            : []),
          {
            id: "pause",
            label: t.gameRooms.pause,
            icon: <PauseIcon />,
            run: act("pause"),
          },
          ...(room.noSaves
            ? []
            : [
                {
                  id: "save",
                  label: t.gameRooms.save,
                  icon: <SaveIcon />,
                  run: act("save"),
                },
              ]),
          {
            id: "archive",
            label: t.gameRooms.archive,
            icon: <ArchiveIcon />,
            run: act("archive"),
          },
        ]
      : room.state === "paused"
        ? [
            {
              id: "resume",
              label: t.gameRooms.resume,
              icon: <PlayIcon />,
              run: act("resume"),
            },
            ...(room.roomId
              ? [
                  {
                    id: "open",
                    label: t.gameRooms.open,
                    icon: <EnterIcon />,
                    to: `/r/${room.roomId}`,
                  },
                ]
              : []),
            ...(room.noSaves
              ? []
              : [
                  {
                    id: "save",
                    label: t.gameRooms.save,
                    icon: <SaveIcon />,
                    run: act("save"),
                  },
                ]),
            {
              id: "archive",
              label: t.gameRooms.archive,
              icon: <ArchiveIcon />,
              run: act("archive"),
            },
            {
              id: "delete",
              label: t.gameRooms.delete,
              icon: <TrashIcon />,
              danger: true,
              run: act("delete"),
            },
          ]
        : room.state === "archived"
          ? [
              {
                id: "start",
                label: t.gameRooms.turnOn,
                icon: <PowerIcon />,
                run: onStart,
              },
              {
                id: "delete",
                label: t.gameRooms.delete,
                icon: <TrashIcon />,
                danger: true,
                run: act("delete"),
              },
            ]
          : [
              {
                id: "restore",
                label: t.gameRooms.restore,
                icon: <RestoreIcon />,
                run: onStart,
              },
              {
                id: "purge",
                label: t.gameRooms.purge,
                icon: <TrashIcon />,
                danger: true,
                run: act("purge"),
              },
            ];
  const favorite: ActionItem = {
    id: room.favorite ? "unfavorite" : "favorite",
    label: room.favorite ? t.gameRooms.unfavorite : t.gameRooms.favorite,
    icon: <StarIcon filled={room.favorite} />,
    run: act(room.favorite ? "unfavorite" : "favorite"),
  };
  // Menu only: what guests see until they pick their own picture.
  const picture: ActionItem = {
    id: "picture",
    label: t.picture.defaultButton,
    icon: <PictureIcon />,
    run: act("picture"),
  };
  const menu = room.state === "trash" ? [...items, favorite] : [...items, favorite, picture];
  const [primary, ...rest] = items;

  const iconButton = (it: ActionItem, isPrimary = false) => {
    const className = `groom-icon${isPrimary ? " is-primary" : ""}${it.danger ? " is-danger" : ""}`;
    return it.to ? (
      <Link
        key={it.id}
        to={it.to}
        className={className}
        aria-label={it.label}
        data-tip={it.label}
      >
        {it.icon}
      </Link>
    ) : (
      <button
        key={it.id}
        type="button"
        className={className}
        aria-label={it.label}
        data-tip={it.label}
        onClick={it.run}
      >
        {it.icon}
      </button>
    );
  };

  return (
    <div className="groom-actions">
      {primary && iconButton(primary, true)}
      {rest.map((it) => iconButton(it))}
      <div className="groom-more" ref={menuRef}>
        <button
          type="button"
          className="groom-icon"
          aria-label={t.gameRooms.more}
          data-tip={t.gameRooms.more}
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          <MoreIcon />
        </button>
        {open && (
          <div className="groom-menu" role="menu">
            {menu.map((it) =>
              it.to ? (
                <Link
                  key={it.id}
                  to={it.to}
                  role="menuitem"
                  className="groom-menu-item"
                >
                  {it.icon}
                  {it.label}
                </Link>
              ) : (
                <button
                  key={it.id}
                  type="button"
                  role="menuitem"
                  className={`groom-menu-item${it.danger ? " is-danger" : ""}`}
                  onClick={() => {
                    setOpen(false);
                    it.run?.();
                  }}
                >
                  {it.icon}
                  {it.label}
                </button>
              ),
            )}
          </div>
        )}
      </div>
    </div>
  );
}

export function StarIcon({ filled }: { filled: boolean }) {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill={filled ? "currentColor" : "none"}
      stroke="currentColor"
      strokeWidth="2"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m12 3 2.7 5.6 6.1.9-4.4 4.3 1 6.1L12 17l-5.4 2.9 1-6.1-4.4-4.3 6.1-.9z" />
    </svg>
  );
}

/** Asks how to start a room again: where it was, from scratch, or a saved game. */
function StartDialog({
  room,
  boxart,
  onCancel,
  onStart,
}: {
  room: ManagedRoom;
  boxart: boolean;
  onCancel: () => void;
  onStart: (s: Start) => void;
}) {
  const art = useThumbnail(room.rom, useThumbKind(), "mini", boxart);
  // A game the emulator cannot save whole only starts from the beginning.
  const resumable = !room.noSaves;
  const options: (Start & { title: string; detail: string; tag: string })[] = [
    ...(resumable && room.autosave
      ? [
          {
            from: "continue" as const,
            slot: 0,
            title: t.gameRooms.fromContinue,
            detail: t.gameRooms.fromContinueDetail,
            tag: "",
          },
        ]
      : []),
    {
      from: "fresh",
      slot: 0,
      title: t.gameRooms.fromFresh,
      detail: t.gameRooms.fromFreshDetail,
      tag: "",
    },
    ...[...(resumable ? room.saves : [])]
      .sort((a, b) => b.slot - a.slot)
      .map((s) => ({
        from: "slot" as const,
        slot: s.slot,
        title: t.gameRooms.fromSlot(s.slot),
        detail: s.name,
        tag: s.at ? new Date(s.at).toLocaleString() : "",
      })),
  ];
  const [choice, setChoice] = useState(0);
  const first = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    first.current?.focus();
    const onKey = (e: KeyboardEvent) => (e.key === "Escape" || e.code === "Escape") && onCancel();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onCancel]);
  return (
    <div
      className="dialog-backdrop"
      onMouseDown={(e) => e.target === e.currentTarget && onCancel()}
    >
      <div
        className="dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="start-title"
      >
        <div className="groom-dialog-head">
          {art && (
            <span className="groom-dialog-art">
              <GameArt src={art} />
            </span>
          )}
          <div className="stack-xxs">
            <h2 id="start-title" className="dialog-title">
              {t.gameRooms.startTitle(room.name)}
            </h2>
            <span className="small muted">{t.gameRooms.startQuestion}</span>
          </div>
        </div>
        <div
          className="groom-options"
          role="radiogroup"
          aria-label={t.gameRooms.startQuestion}
        >
          {options.map((o, i) => (
            <button
              key={`${o.from}-${o.slot}`}
              ref={i === 0 ? first : undefined}
              type="button"
              role="radio"
              aria-checked={choice === i}
              className={`groom-option${choice === i ? " is-on" : ""}`}
              onClick={() => setChoice(i)}
            >
              <i />
              <span className="stack-xxs">
                <strong>{o.title}</strong>
                {o.detail && <span className="small muted">{o.detail}</span>}
              </span>
              {o.tag && <span className="small faint">{o.tag}</span>}
            </button>
          ))}
        </div>
        {!resumable && <p className="small muted">{t.gameRooms.noSaves}</p>}
        <div className="dialog-actions">
          <button
            type="button"
            className="button button-primary"
            onClick={() => onStart(options[choice]!)}
          >
            {t.gameRooms.confirmStart}
          </button>
          <button
            type="button"
            className="button button-secondary"
            onClick={onCancel}
          >
            {t.gameRooms.cancel}
          </button>
        </div>
      </div>
    </div>
  );
}
