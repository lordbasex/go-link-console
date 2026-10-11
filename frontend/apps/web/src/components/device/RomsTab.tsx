// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import {
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type FormEvent,
} from "react";
import { Link } from "react-router-dom";
import { formatBytes, romFile, romPlayable, type DeviceRom } from "@go-link/shared";
import { t } from "../../i18n";
import { useSignal } from "../../signal/SignalProvider";
import { romCheckText } from "../romCheck";
import { KIND_COLOR, KIND_LABEL, kindOf, type Kind } from "./romKinds";
import { useThumbKind, useThumbnail } from "./useThumbnail";
import { SkeletonCards, SkeletonRows } from "../ui/Skeleton";
import { useRomPages } from "./useLibrary";
import { OwnBadge, ownControlsText } from "./OwnBadge";
import { Select } from "../ui/Select";
import { GameArt } from "../ui/GameArt";

type View = "cards" | "list";
type Sort = "name" | "size" | "year" | "status";
type Filter = "all" | Kind;

const VIEW_KEY = "go-link.roms-view";
const KIND_ORDER: Kind[] = [
  "runs",
  "missing",
  "unsupported",
  "broken",
  "bios",
  "unchecked",
];

function readView(): View {
  try {
    return window.localStorage.getItem(VIEW_KEY) === "list" ? "list" : "cards";
  } catch {
    return "cards";
  }
}

const titleOf = (r: DeviceRom) => r.title || r.name;
const initials = (s: string) =>
  s
    .replace(/[^A-Za-z0-9 ]/g, "")
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0])
    .join("")
    .toUpperCase();

/** The "ROMs" tab of My device: the library as cards or a list, and adding ROMs. */
export function RomsTab() {
  const { linkedDevice } = useSignal();
  const library = linkedDevice.status?.library;
  const summary = library?.summary;
  const [view, setView] = useState<View>(readView);
  const [filter, setFilter] = useState<Filter>("all");
  const [sort, setSort] = useState<Sort>("name");
  const [query, setQuery] = useState("");
  // The device searches, filters and sorts; the sets come a page at a time.
  const { roms: shown, total: matching, more, sentinel, loadMore } = useRomPages(
    { q: query, filter, sort },
    { enabled: !!library },
  );
  const counts = summary?.kinds ?? { runs: 0, missing: 0, unsupported: 0, broken: 0, bios: 0, unchecked: 0 };
  const all = summary?.total ?? 0;

  const pickView = (v: View) => {
    setView(v);
    try {
      window.localStorage.setItem(VIEW_KEY, v);
    } catch {
      // storage blocked: the choice lasts for this page
    }
  };

  const chips: { id: Filter; label: string; count: number; color: string }[] = [
    {
      id: "all",
      label: t.dash.filterAll,
      count: all,
      color: "var(--color-text-faint)",
    },
    ...KIND_ORDER.filter(
      (k) => counts[k] > 0 || k === "runs" || k === "missing",
    ).map((k) => ({
      id: k as Filter,
      label: KIND_LABEL[k](),
      count: counts[k],
      color: KIND_COLOR[k],
    })),
  ];
  const bytes = summary?.bytes ?? 0;
  const ready = summary?.playable ?? 0;

  return (
    <div className="roms">
      <section className="roms-main" aria-label={t.dash.romsTitle}>
        <div className="roms-toolbar">
          <label className="dash-search roms-search">
            <span className="visually-hidden">{t.dash.search}</span>
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
              type="search"
              placeholder={t.roms.search}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
          </label>
          <span className="roms-sort">
            <label id="roms-sort-label" htmlFor="roms-sort">
              {t.roms.sort}
            </label>
            <Select<Sort>
              id="roms-sort"
              labelId="roms-sort-label"
              value={sort}
              onChange={setSort}
              options={[
                { value: "name", label: t.roms.sortName },
                { value: "size", label: t.roms.sortSize },
                { value: "year", label: t.roms.sortYear },
                { value: "status", label: t.roms.sortStatus },
              ]}
            />
          </span>
          <div className="roms-toolbar-end">
            <span className="small faint">
              {t.roms.showing(shown ? matching : 0, all)}
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

        <div className="roms-chips" role="group" aria-label={t.dash.filter}>
          {chips.map((c) => (
            <button
              key={c.id}
              type="button"
              aria-pressed={filter === c.id}
              className={filter === c.id ? "is-on" : ""}
              onClick={() => setFilter(c.id)}
            >
              <i style={{ background: c.color }} />
              {c.label}
              <span>{c.count}</span>
            </button>
          ))}
        </div>

        {!library || (all > 0 && !shown) ? (
          view === "cards" ? <SkeletonCards label={t.common.loading} tall /> : <SkeletonRows label={t.common.loading} />
        ) : all === 0 ? (
          <p className="muted small-plus">{t.linked.libraryEmpty}</p>
        ) : !shown || shown.length === 0 ? (
          <p className="muted small-plus">{t.dash.noMatch}</p>
        ) : view === "cards" ? (
          <ul className="roms-cards">
            {shown.map((r) => (
              <RomCard key={r.name} rom={r} />
            ))}
          </ul>
        ) : (
          <RomTable roms={shown} />
        )}
        {library && more && (
          <div ref={sentinel} className="list-more">
            <button type="button" className="button button-secondary" onClick={loadMore}>
              {t.common.showMore}
            </button>
          </div>
        )}
      </section>

      <aside className="roms-side stack-md">
        <div className="card dash-card stack-sm">
          <h2 className="card-title">{t.roms.library}</h2>
          <div className="dash-kpi-value">
            <span className="dash-kpi-number">
              {bytes ? formatBytes(bytes).split(" ")[0] : "0"}
            </span>
            <span className="dash-kpi-unit">
              {`${bytes ? formatBytes(bytes).split(" ")[1] : "MB"} · ${all} ${t.dash.sets}`}
            </span>
          </div>
          <div className="dash-stack-bar">
            {KIND_ORDER.map(
              (k) =>
                counts[k] > 0 && (
                  <span
                    key={k}
                    style={{
                      width: `${(counts[k] / Math.max(all, 1)) * 100}%`,
                      background: KIND_COLOR[k],
                    }}
                  />
                ),
            )}
          </div>
          <div className="dash-row small">
            <span className="muted">{t.roms.ready}</span>
            <span className="mono is-voice-text">
              {t.roms.readyValue(ready, all)}
            </span>
          </div>
        </div>
        <AddRoms />
      </aside>
    </div>
  );
}

function RomCard({ rom }: { rom: DeviceRom }) {
  const kind = kindOf(rom);
  const title = titleOf(rom);
  const playable = romPlayable(rom);
  const thumbKind = useThumbKind();
  const art = useThumbnail(rom.name, thumbKind, "card", rom.thumbs[thumbKind]);
  return (
    <li
      className="roms-card"
      style={{ ["--kind-color" as string]: KIND_COLOR[kind] }}
      title={romCheckText(rom.check) || undefined}
    >
      <div className={`roms-screen${playable ? " is-playable" : ""}${art ? " has-art" : ""}`}>
        {art ? (
          <GameArt className="roms-art" src={art} />
        ) : (
          <span className="roms-marquee">{title.toUpperCase()}</span>
        )}
        <span className="roms-float-badge">{KIND_LABEL[kind]()}</span>
      </div>
      <div className="roms-card-body">
        <span className="roms-card-title">
          {title}
          {rom.own && <OwnBadge />}
        </span>
        <span className="small muted">
          {[rom.year, rom.maker].filter(Boolean).join(" · ") ||
            t.roms.unknownGame}
        </span>
        {rom.own && rom.description && (
          <span className="small muted">{rom.description}</span>
        )}
        {rom.own && ownControlsText(rom) && (
          <span className="small faint">{ownControlsText(rom)}</span>
        )}
        <div className="roms-card-foot">
          <span className="small faint mono">
            {rom.name} · {formatBytes(rom.size)}
          </span>
          {playable && <PlayLink rom={rom} />}
        </div>
      </div>
    </li>
  );
}

function RomTable({ roms }: { roms: DeviceRom[] }) {
  return (
    <div className="roms-table-wrap">
      <table className="roms-table">
        <thead>
          <tr>
            <th scope="col">{t.roms.colGame}</th>
            <th scope="col" className="roms-col-set">
              {t.roms.colSet}
            </th>
            <th scope="col" className="roms-col-year">
              {t.roms.colYear}
            </th>
            <th scope="col" className="roms-col-maker">
              {t.roms.colMaker}
            </th>
            <th scope="col" className="roms-num">
              {t.roms.colSize}
            </th>
            <th scope="col">{t.roms.colStatus}</th>
            <th scope="col" className="roms-num">
              <span className="visually-hidden">{t.roms.colActions}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {roms.map((r) => {
            const kind = kindOf(r);
            const title = titleOf(r);
            return (
              <tr
                key={r.name}
                style={{ ["--kind-color" as string]: KIND_COLOR[kind] }}
                title={romCheckText(r.check) || undefined}
              >
                <td>
                  <div className="roms-cell-game">
                    <MiniArt rom={r} fallback={initials(title)} />
                    <span className="strong">
                      {title}
                      {r.own && <OwnBadge />}
                    </span>
                  </div>
                </td>
                <td className="roms-col-set mono small muted">{romFile(r)}</td>
                <td className="roms-col-year">{r.year || "–"}</td>
                <td className="roms-col-maker">{r.maker || "–"}</td>
                <td className="roms-num mono small">{formatBytes(r.size)}</td>
                <td>
                  <span className={`dash-badge is-${kind}`}>
                    {KIND_LABEL[kind]()}
                  </span>
                </td>
                <td className="roms-num">
                  {romPlayable(r) && <PlayLink rom={r} />}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

/** The small Boxart of a list row, or the game's initials. */
export function MiniArt({ rom, fallback }: { rom: DeviceRom; fallback: string }) {
  const thumbKind = useThumbKind();
  const art = useThumbnail(rom.name, thumbKind, "mini", rom.thumbs[thumbKind]);
  return (
    <span className={`roms-mini-screen${art ? " has-art" : ""}`} aria-hidden="true">
      {art ? <GameArt src={art} /> : fallback}
    </span>
  );
}

/** Opens "Create room" with this game already chosen. */
function PlayLink({ rom }: { rom: DeviceRom }) {
  return (
    <Link
      to={`/create?rom=${encodeURIComponent(rom.name)}`}
      className="roms-play"
      aria-label={t.roms.playLabel(titleOf(rom))}
    >
      <svg width="14" height="14" viewBox="0 0 24 24" aria-hidden="true">
        <path d="M7 5v14l12-7z" fill="currentColor" />
      </svg>
      <span>{t.roms.play}</span>
    </Link>
  );
}

/** Drop zone, ROM folder, and where ROMs come from. */
function AddRoms() {
  const { linkedDevice, hostLink, onDeviceMessage } = useSignal();
  const library = linkedDevice.status?.library;
  const [dirValue, setDirValue] = useState<string | null>(null);
  const [dirError, setDirError] = useState("");
  const [dropOver, setDropOver] = useState(false);
  const [uploadText, setUploadText] = useState("");
  const [uploading, setUploading] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const waiters = useRef(
    new Map<string, (r: { ok: boolean; error?: string }) => void>(),
  );

  useEffect(
    () =>
      onDeviceMessage((msg) => {
        const m = msg as {
          type?: string;
          id?: string;
          ok?: boolean;
          error?: string;
          code?: string;
        };
        if (m.type === "upload_result" && m.id) {
          // "not_rom": a Willy Maker project or AI pack, named in the user's language (T-21)
          waiters.current.get(m.id)?.({ ok: m.ok === true, error: m.code === "not_rom" ? t.linked.notRom : m.error });
          waiters.current.delete(m.id);
        } else if (m.type === "roms_dir_result") {
          setDirError(m.ok ? "" : (m.error ?? t.linked.folderError));
          if (m.ok) setDirValue(null);
        }
      }),
    [onDeviceMessage],
  );

  const saveDir = (e: FormEvent) => {
    e.preventDefault();
    if (dirValue !== null)
      hostLink?.stream.sendControl({
        type: "set_roms_dir",
        dir: dirValue.trim(),
      });
  };

  // Files go to the device over WebRTC ("files" channel), one at a time.
  const upload = async (files: FileList | File[]) => {
    const stream = hostLink?.stream;
    // ROM sets, and PNG/JPG pictures that become Boxart thumbnails.
    const all = [...files];
    // a Willy Maker project or AI pack is the game's sources, not a ROM (T-21)
    const sources = all.filter((f) => /\.(willy|ai-pack)\.zip$/i.test(f.name));
    const list = all.filter(
      (f) => /\.(zip|png|jpe?g)$/i.test(f.name) && !sources.includes(f),
    );
    if (sources.length && list.length === 0) {
      setUploadText(`${sources.map((f) => f.name).join(", ")}: ${t.linked.notRom}`);
      return;
    }
    if (!stream || list.length === 0) {
      setUploadText(t.linked.dropOnlyZip);
      return;
    }
    setUploading(true);
    let ok = 0;
    const failed: string[] = [];
    for (const [i, file] of list.entries()) {
      const id = `${Date.now()}-${i}`;
      const result = new Promise<{ ok: boolean; error?: string }>((resolve) =>
        waiters.current.set(id, resolve),
      );
      try {
        await stream.sendFile(id, file, file.name, (sent, total) =>
          setUploadText(
            t.linked.dropProgress(
              i + 1,
              list.length,
              file.name,
              Math.round((sent / total) * 100),
            ),
          ),
        );
        const r = await result;
        if (r.ok) ok++;
        else failed.push(`${file.name} (${r.error ?? "?"})`);
      } catch (err) {
        waiters.current.delete(id);
        failed.push(
          `${file.name} (${err instanceof Error ? err.message : "?"})`,
        );
      }
    }
    setUploading(false);
    for (const f of sources) failed.push(`${f.name} (${t.linked.notRom})`);
    setUploadText(t.linked.dropDone(ok, list.length + sources.length, failed));
  };

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDropOver(false);
    if (!uploading) void upload(e.dataTransfer.files);
  };

  return (
    <>
      <div
        className={`drop-zone dash-drop${dropOver ? " is-over" : ""}`}
        role="button"
        tabIndex={0}
        aria-disabled={uploading}
        onClick={() => !uploading && fileInput.current?.click()}
        onKeyDown={(e) =>
          (e.key === "Enter" || e.key === " ") &&
          !uploading &&
          fileInput.current?.click()
        }
        onDragOver={(e) => {
          e.preventDefault();
          setDropOver(true);
        }}
        onDragLeave={() => setDropOver(false)}
        onDrop={onDrop}
      >
        <svg
          width="30"
          height="30"
          viewBox="0 0 24 24"
          fill="none"
          stroke="var(--color-accent)"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden="true"
        >
          <path d="M12 16V4M7 9l5-5 5 5M4 20h16" />
        </svg>
        <strong className="small-plus">{t.linked.dropTitle}</strong>
        <span className="small muted">{t.linked.dropHint}</span>
        <input
          ref={fileInput}
          type="file"
          accept=".zip,.png,.jpg,.jpeg"
          multiple
          hidden
          onChange={(e) => {
            if (e.target.files) void upload(e.target.files);
            e.target.value = "";
          }}
        />
      </div>
      {uploadText && (
        <p className="small muted" aria-live="polite">
          {uploadText}
        </p>
      )}

      {library && (
        <form className="card dash-card stack-sm" onSubmit={saveDir}>
          <label htmlFor="roms-dir" className="card-title">
            {t.linked.folder}
          </label>
          <input
            id="roms-dir"
            className="input input-dark input-small input-mono"
            spellCheck={false}
            autoComplete="off"
            value={dirValue ?? library.dir}
            placeholder="/Volumes/Disk/ROMS/MAME-2003-Plus"
            onChange={(e) => setDirValue(e.target.value)}
          />
          <button
            type="submit"
            className="button button-secondary"
            disabled={dirValue === null || dirValue.trim() === library.dir}
          >
            {t.linked.useFolder}
          </button>
          {dirError && (
            <p className="form-error small" role="alert">
              {dirError}
            </p>
          )}
        </form>
      )}

      {library && <ThumbnailSettings />}

      <div className="card dash-card stack-sm">
        <h2 className="card-title">{t.roms.sourceTitle}</h2>
        <p className="small-plus muted">{t.roms.sourceText}</p>
        <p className="small faint">{t.roms.sourceRights}</p>
      </div>
    </>
  );
}

const KINDS = ["boxart", "title", "snap"] as const;

/**
 * The device's thumbnail settings, like its window's Settings: which
 * picture everyone sees for each game, and the folder it reads them from.
 */
function ThumbnailSettings() {
  const { linkedDevice, hostLink, onDeviceMessage } = useSignal();
  const library = linkedDevice.status?.library;
  const [dirValue, setDirValue] = useState<string | null>(null);
  const [error, setError] = useState("");

  useEffect(
    () =>
      onDeviceMessage((msg) => {
        const m = msg as { type?: string; ok?: boolean; error?: string };
        if (m.type === "thumbnails_result") {
          setError(m.ok ? "" : (m.error ?? t.thumbs.error));
          if (m.ok) setDirValue(null);
        }
      }),
    [onDeviceMessage],
  );
  if (!library) return null;
  const send = (change: { kind?: string; dir?: string }) =>
    hostLink?.stream.sendControl({ type: "set_thumbnails", ...change });
  const counts = KINDS.map((k) => library.summary.thumbs[k]);

  return (
    <form
      className="card dash-card stack-sm"
      onSubmit={(e) => {
        e.preventDefault();
        if (dirValue !== null) send({ dir: dirValue.trim() || "default" });
      }}
    >
      <h2 className="card-title">{t.thumbs.title}</h2>
      <p className="small muted">{t.thumbs.showText}</p>
      <div className="dash-segmented" role="group" aria-label={t.thumbs.show}>
        {KINDS.map((k, i) => (
          <button
            key={k}
            type="button"
            aria-pressed={library.thumbKind === k}
            className={library.thumbKind === k ? "is-on" : ""}
            onClick={() => send({ kind: k })}
          >
            {`${t.thumbs.kinds[i]} · ${counts[i]}`}
          </button>
        ))}
      </div>
      <label htmlFor="thumbs-dir" className="small muted">
        {t.thumbs.folder}
      </label>
      <input
        id="thumbs-dir"
        className="input input-dark input-small input-mono"
        spellCheck={false}
        autoComplete="off"
        value={dirValue ?? library.thumbnailsDir}
        onChange={(e) => setDirValue(e.target.value)}
      />
      <div className="chip-row">
        <button
          type="submit"
          className="button button-secondary"
          disabled={dirValue === null || dirValue.trim() === library.thumbnailsDir}
        >
          {t.thumbs.useFolder}
        </button>
        <button type="button" className="button button-secondary" onClick={() => send({ dir: "default" })}>
          {t.thumbs.defaultFolder}
        </button>
      </div>
      <p className="small faint">{t.thumbs.help}</p>
      {error && (
        <p className="form-error small" role="alert">
          {error}
        </p>
      )}
    </form>
  );
}
