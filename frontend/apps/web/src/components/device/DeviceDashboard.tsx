// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useState, type ReactNode } from "react";
import { Link, NavLink, useParams } from "react-router-dom";
import { FACTORY_RESET, formatBytes, parseFactoryReset } from "@go-link/shared";
import { Chip, HeroTile, PageHero } from "../ui/PageHero";
import { t } from "../../i18n";
import { useSignal } from "../../signal/SignalProvider";
import { VideoQualityCard } from "./VideoQualityCard";
import { MonitorIcon, PictureIcon, TestCardIcon, UserPlusIcon } from "../Icons";
import { RoomPictureDialog } from "../RoomPictureDialog";
import { InviteDialog } from "../InviteDialog";
import { ConfirmDialog } from "../RemapDialog";
import { AreaChart, Ring, Sparkline } from "./charts";
import { HISTORY_SIZE, useHistory } from "./useHistory";
import { KIND_COLOR, KIND_LABEL, type Kind } from "./romKinds";
import { RomsTab } from "./RomsTab";
import { HistoryTab } from "./HistoryTab";
import { NetworkReport } from "./NetworkReport";
import { SkeletonCards } from "../ui/Skeleton";

const tabClass = ({ isActive }: { isActive: boolean }) =>
  `dash-tab${isActive ? " is-active" : ""}`;

const RANGES = [30, 60, HISTORY_SIZE] as const; // samples: 1, 2 and 5 minutes

const round = (n: number) => Math.round(n);
const shortId = (id: string) =>
  id.length > 12 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id;
const hostOf = (url: string) => {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
};

/** Live dashboard of the linked device, fed over the WebRTC data link. */
export function DeviceDashboard({
  tab = "overview",
}: {
  tab?: "overview" | "roms" | "history" | "network";
}) {
  const { linkedDevice, unlinkDevice, hostLink, sendToDevice, onDeviceMessage, panel } = useSignal();
  const { status, rttMs, path, state } = linkedDevice;
  const hw = status?.system?.hardware;
  const usage = status?.system?.usage;
  const cores = Math.max(hw?.cores ?? 1, 1);
  // The process figure is per core (100 = one core): scale it to the whole machine.
  const procMachine = usage ? usage.process_cpu_percent / cores : 0;
  const memTotal = hw?.mem_total ?? 0;

  const history = useHistory(
    status?.system?.sampled_at,
    usage
      ? {
          sys: usage.cpu_percent,
          proc: procMachine,
          lat: rttMs ?? 0,
          up: ((usage.net_sent_bps ?? 0) * 8) / 1e6,
          down: ((usage.net_recv_bps ?? 0) * 8) / 1e6,
        }
      : null,
  );
  const [range, setRange] = useState<number>(60);
  const sys = (history.sys ?? []).slice(-range);
  const proc = (history.proc ?? []).slice(-range);
  const avg = sys.length ? sys.reduce((a, b) => a + b, 0) / sys.length : 0;
  const peak = sys.length ? Math.max(...sys) : 0;
  const cpuNow = usage?.cpu_percent ?? 0;
  const memPct = memTotal ? ((usage?.mem_used ?? 0) / memTotal) * 100 : 0;
  const upNow = history.up?.at(-1) ?? 0;
  const downNow = history.down?.at(-1) ?? 0;
  const netMax = Math.max(1, ...(history.up ?? []), ...(history.down ?? []));

  const room = status?.room;
  const [inviteOpen, setInviteOpen] = useState(false);
  const [pictureOpen, setPictureOpen] = useState(false);
  const [resetAsk, setResetAsk] = useState(false);
  const [resetError, setResetError] = useState("");
  // On success the device unlinks every browser by itself.
  useEffect(
    () =>
      onDeviceMessage((msg) => {
        const r = parseFactoryReset(msg);
        if (r && !r.ok) setResetError(t.reset.failed(r.error));
      }),
    [onDeviceMessage],
  );
  const library = status?.library;
  // The library in numbers; its sets come in pages (RomsTab).
  const summary = library?.summary;
  const romCount = summary?.total ?? 0;
  const romBytes = summary?.bytes ?? 0;

  const secs = range * 2;
  const xLabels = [4, 3, 2, 1, 0].map((i) => {
    const s = round((secs * i) / 4);
    return i === 0
      ? t.dash.now
      : `-${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
  });

  return (
    <div className="dash">
      <PageHero
        tile={
          <HeroTile
            status={
              state === "connected"
                ? "live"
                : state === "failed"
                  ? "failed"
                  : "idle"
            }
          >
            <MonitorIcon size={38} />
          </HeroTile>
        }
        eyebrow={t.pairing.eyebrow}
        title={hw?.hostname?.replace(/\.local$/i, "") || t.dash.unnamed}
        chips={
          <>
            <Chip
              tone={
                state === "failed"
                  ? "danger"
                  : state === "connected"
                    ? "live"
                    : undefined
              }
              dot
            >
              {state === "failed"
                ? t.dash.failed
                : state === "connected"
                  ? t.dash.live
                  : t.dash.connecting}
            </Chip>
            {hw && <Chip>{`${hw.platform || hw.os} · ${hw.arch}`}</Chip>}
            {hw && (
              <Chip>{`${hw.cpu_model || "CPU"} · ${t.dash.threads(hw.cores)}`}</Chip>
            )}
            {status && (
              <Chip mono>{`device ${shortId(status.device_id)}`}</Chip>
            )}
            {status?.version && <Chip mono>{`go-link ${status.version}`}</Chip>}
          </>
        }
        actions={
          <>
            <button
              type="button"
              className="button button-danger"
              onClick={unlinkDevice}
            >
              {panel ? t.panel.logout : t.linked.unlink}
            </button>
            {status && (
              <button
                type="button"
                className="button button-secondary"
                onClick={() => {
                  setResetError("");
                  setResetAsk(true);
                }}
              >
                {t.reset.button}
              </button>
            )}
            {/* The test pattern room: a permanent check of video, sound,
                controllers and latency, like a VoIP echo test. */}
            {room?.room_id ? (
              <>
                <button
                  type="button"
                  className="icon-button tip-below"
                  aria-label={t.dash.testInvite}
                  data-tip={t.dash.testInvite}
                  onClick={() => setInviteOpen(true)}
                >
                  <UserPlusIcon />
                </button>
                <button
                  type="button"
                  className="icon-button tip-below"
                  aria-label={t.picture.defaultButton}
                  data-tip={t.picture.defaultButton}
                  onClick={() => setPictureOpen(true)}
                >
                  <PictureIcon />
                </button>
                <Link
                  to={`/r/${room.room_id}`}
                  className="button button-primary tip-below"
                  data-tip={t.dash.testPatternHint}
                >
                  <TestCardIcon size={18} />
                  {t.dash.testPattern}
                </Link>
              </>
            ) : (
              <button type="button" className="button button-primary" disabled>
                <TestCardIcon size={18} />
                {t.dash.testPattern}
              </button>
            )}
          </>
        }
      />

      {state === "failed" && (
        <p className="notice" role="status">
          {t.dash.failedText}
        </p>
      )}
      {status?.update && (
        <div className="help-box dash-update" role="status">
          <p>{t.dash.update(status.update.latest, status.version)}</p>
          <a className="button button-primary button-small" href={status.update.url} target="_blank" rel="noopener">
            {t.dash.updateDownload}
          </a>
        </div>
      )}


      <nav className="dash-tabs" aria-label={t.dash.sections}>
        <NavLink to="/device" end className={tabClass}>
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <path d="M3 12h4l3-8 4 16 3-8h4" />
          </svg>
          {t.dash.overview}
        </NavLink>
        <NavLink to="/device/roms" className={tabClass}>
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <rect x="3" y="4" width="18" height="16" rx="2" />
            <path d="M7 8h10M7 12h10M7 16h6" />
          </svg>
          {t.dash.romsTitle}
          {library && <span className="dash-tab-count">{romCount}</span>}
        </NavLink>
        <NavLink to="/device/history" className={tabClass}>
          <svg
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <path d="M3 12a9 9 0 1 0 3-6.7L3 8" />
            <path d="M3 3v5h5M12 7v5l3 2" />
          </svg>
          {t.history.tab}
        </NavLink>
      </nav>

      {tab === "roms" ? (
        <RomsTab />
      ) : tab === "history" ? (
        <HistoryTab />
      ) : tab === "network" ? (
        <NetworkRoute />
      ) : !status ? (
        state !== "failed" && <SkeletonCards cards={4} label={t.dash.waiting} />
      ) : (
        <>
          <section className="dash-kpis" aria-label={t.dash.figures}>
            <Kpi
              label={t.dash.cpuSystem}
              color="var(--color-accent)"
              value={usage ? String(pct1(cpuNow)) : "–"}
              unit="%"
              foot={t.dash.peak(pct1(peak))}
            >
              <Sparkline
                values={(history.sys ?? []).slice(-30)}
                color="var(--color-accent)"
                max={100}
              />
            </Kpi>
            <Kpi
              label={t.dash.cpuGoLink}
              color="var(--color-voice)"
              value={usage ? String(round(procMachine)) : "–"}
              unit="%"
              foot={t.dash.cpuGoLinkHint}
            >
              <Sparkline
                values={(history.proc ?? []).slice(-30)}
                color="var(--color-voice)"
                max={Math.max(10, ...(history.proc ?? []).slice(-30))}
              />
            </Kpi>
            <Kpi
              label={t.dash.memory}
              color="var(--color-p4)"
              value={usage ? (usage.mem_used / 1024 ** 3).toFixed(1) : "–"}
              unit={memTotal ? `/ ${formatBytes(memTotal)}` : ""}
              foot={t.dash.memoryHint(
                round(memPct),
                formatBytes(usage?.process_rss),
              )}
            >
              <div className="dash-bar">
                <span
                  style={{ width: `${memPct}%`, background: "var(--color-p4)" }}
                />
              </div>
            </Kpi>
            <Kpi
              label={t.dash.latency}
              color="var(--color-p3)"
              value={rttMs === null ? "–" : rttMs < 1 ? "<1" : String(rttMs)}
              unit="ms"
              foot={t.dash.latencyHint(path)}
            >
              <Sparkline
                values={(history.lat ?? []).slice(-40)}
                color="var(--color-p3)"
                max={Math.max(20, ...(history.lat ?? []).slice(-40))}
                fill={false}
              />
            </Kpi>
            <Kpi
              label={t.dash.players}
              color="var(--color-accent)"
              value={room ? String(room.players ?? 0) : "–"}
              unit={room ? `/ ${room.max_players ?? 4}` : ""}
              foot={
                room
                  ? t.dash.playersHint(
                      room.spectators ?? room.viewers,
                      room.queue ?? 0,
                    )
                  : t.dash.noRoom
              }
            >
              <Seats
                players={room?.players ?? 0}
                max={room?.max_players ?? 4}
              />
            </Kpi>
          </section>

          <section className="dash-charts">
            <div className="card dash-card stack-md">
              <div className="dash-card-head">
                <div className="dash-card-title-row">
                  <h2 className="card-title">{t.dash.cpuTitle}</h2>
                  <span className="dash-live-badge">
                    <span className="dash-live-dot" />
                    {t.dash.liveBadge}
                  </span>
                </div>
                <div className="dash-card-title-row">
                  <span className="dash-legend">
                    <span style={{ background: "var(--color-accent)" }} />
                    {t.dash.system}
                  </span>
                  <span className="dash-legend">
                    <span style={{ background: "var(--color-voice)" }} />
                    go-link
                  </span>
                  <div
                    className="dash-segmented"
                    role="group"
                    aria-label={t.dash.range}
                  >
                    {RANGES.map((n, i) => (
                      <button
                        key={n}
                        type="button"
                        aria-pressed={range === n}
                        className={range === n ? "is-on" : ""}
                        onClick={() => setRange(n)}
                      >
                        {t.dash.ranges[i]}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
              <div className="dash-chart-wrap">
                <div className="dash-y-axis" aria-hidden="true">
                  <span>100%</span>
                  <span>75%</span>
                  <span>50%</span>
                  <span>25%</span>
                  <span>0%</span>
                </div>
                <div className="stack-xs grow">
                  <AreaChart
                    label={t.dash.cpuTitle}
                    slots={range}
                    average={sys.length ? avg : undefined}
                    series={[
                      { values: sys, color: "var(--color-accent)" },
                      { values: proc, color: "var(--color-voice)" },
                    ]}
                  />
                  <div className="dash-x-axis" aria-hidden="true">
                    {xLabels.map((l) => (
                      <span key={l}>{l}</span>
                    ))}
                  </div>
                </div>
              </div>
              <div className="dash-stats">
                <Stat label={t.dash.average} value={`${avg.toFixed(1)}%`} />
                <Stat label={t.dash.peakLabel} value={`${pct1(peak)}%`} />
                <Stat
                  label={t.dash.share}
                  value={
                    // Below 1% of the machine the two samples (machine and
                    // process) are too small to split: the process could even
                    // read higher than the whole machine.
                    cpuNow >= 1 ? `${round(Math.min(100, (procMachine / cpuNow) * 100))}%` : "–"
                  }
                  tone="voice"
                />
                <Stat
                  label={t.dash.headroom}
                  value={t.dash.threads(
                    Math.max(
                      0,
                      Math.round(cores * (1 - cpuNow / 100) * 10) / 10,
                    ),
                  )}
                  tone="accent"
                />
              </div>
            </div>

            <div className="stack-md">
              <div className="card dash-card stack-md">
                <h2 className="card-title">{t.dash.memory}</h2>
                <div className="dash-ring-row">
                  <Ring
                    fraction={memPct / 100}
                    color="var(--color-p4)"
                    center={`${round(memPct)}%`}
                    caption={t.dash.inUse}
                    slice={
                      memTotal
                        ? {
                            fraction: (usage?.process_rss ?? 0) / memTotal,
                            color: "var(--color-voice)",
                          }
                        : undefined
                    }
                  />
                  <dl className="dash-legend-list">
                    <LegendRow
                      color="var(--color-p4)"
                      label={t.dash.used}
                      value={formatBytes(usage?.mem_used)}
                    />
                    <LegendRow
                      color="var(--color-voice)"
                      label="go-link"
                      value={formatBytes(usage?.process_rss)}
                    />
                    <LegendRow
                      color="var(--color-divider)"
                      label={t.dash.free}
                      value={
                        memTotal
                          ? formatBytes(memTotal - (usage?.mem_used ?? 0))
                          : "–"
                      }
                    />
                  </dl>
                </div>
              </div>
              <div className="card dash-card stack-sm grow">
                <div className="dash-card-head">
                  <h2 className="card-title">{t.dash.network}</h2>
                  <span className="small faint">{t.dash.networkHint}</span>
                </div>
                <div className="dash-net">
                  <div className="stack-xxs">
                    <span className="small muted">↑ {t.dash.out}</span>
                    <span className="dash-net-value is-accent">
                      {upNow.toFixed(1)} <small>Mbps</small>
                    </span>
                  </div>
                  <div className="stack-xxs">
                    <span className="small muted">↓ {t.dash.in}</span>
                    <span className="dash-net-value is-voice">
                      {downNow.toFixed(1)} <small>Mbps</small>
                    </span>
                  </div>
                </div>
                <div className="dash-net-chart">
                  <Sparkline
                    values={history.up ?? []}
                    color="var(--color-accent)"
                    max={netMax}
                    height={84}
                  />
                  <Sparkline
                    values={history.down ?? []}
                    color="var(--color-voice)"
                    max={netMax}
                    height={84}
                    fill={false}
                    dashed
                  />
                </div>
              </div>
            </div>
          </section>

          <section className="dash-cards">
            {spaceCard()}
            {libraryCard()}
            {storageCard()}
            <VideoQualityCard />
            <div className="card dash-card stack-md">
              <h2 className="card-title">{t.dash.connection}</h2>
              <div
                className={`dash-path${path === "relay" ? " is-relay" : path === "direct" ? " is-direct" : ""}`}
              >
                <svg
                  width="24"
                  height="24"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  aria-hidden="true"
                >
                  <path d="M5 12h14M13 6l6 6-6 6" />
                </svg>
                <div className="stack-xxs">
                  <strong>
                    {path === "direct"
                      ? t.dash.direct
                      : path === "relay"
                        ? t.dash.relay
                        : t.dash.pathUnknown}
                  </strong>
                  {path && (
                    <span className="small">
                      {path === "direct" ? t.dash.directHint : t.dash.relayHint}
                    </span>
                  )}
                </div>
              </div>
              <dl className="dash-facts">
                <dt>{t.dash.signaling}</dt>
                <dd className="mono">
                  {status ? hostOf(status.signal_url) : "–"}
                </dd>
                <dt>{t.dash.turn}</dt>
                <dd className="mono">
                  {status?.ice_urls[0]
                    ? status.ice_urls[0]
                        .replace(/^(stun|turns?):/, "")
                        .split("?")[0]
                    : t.linked.none}
                </dd>
                <dt>{t.dash.linkedOnline}</dt>
                <dd className="mono">{status?.linked_browsers ?? "–"}</dd>
                <dt>{t.dash.version}</dt>
                <dd className="mono">{status?.version || "–"}</dd>
                <dt>{t.dash.deviceId}</dt>
                <dd className="mono" title={status?.device_id}>
                  {status ? shortId(status.device_id) : "–"}
                </dd>
              </dl>
            </div>
          </section>
        </>
      )}
      {resetAsk && (
        <ConfirmDialog
          title={t.reset.title}
          text={t.reset.text}
          confirm={t.reset.confirm}
          onCancel={() => setResetAsk(false)}
          onConfirm={() => {
            setResetAsk(false);
            sendToDevice(FACTORY_RESET);
          }}
        />
      )}
      {resetError && (
        <p className="notice small" role="alert">
          {resetError}
        </p>
      )}
      {pictureOpen && room?.room_id && (
        <RoomPictureDialog
          id="test"
          name={t.dash.testPattern}
          current={room.picture ?? null}
          send={sendToDevice}
          onClose={() => setPictureOpen(false)}
        />
      )}
      {inviteOpen && room?.room_id && (
        <InviteDialog
          room={{ id: "test", name: t.dash.testPattern, invite: room.invite ?? "", inviteCode: room.invite_code ?? "", groupInvite: room.groupInvite, watchInvite: room.watchInvite }}
          fallbackUrl={`${window.location.origin}/r/${room.room_id}`}
          onAction={(action) => sendToDevice({ type: "room_action", id: "test", action })}
          onClose={() => setInviteOpen(false)}
        />
      )}
    </div>
  );

  function spaceCard() {
    const parts = [
      { label: t.dash.roms, bytes: romBytes, color: "var(--color-accent)" },
      {
        label: t.dash.thumbnails,
        bytes: library?.thumbnailsBytes ?? 0,
        color: "var(--color-voice)",
      },
      {
        label: t.dash.saves,
        bytes: status?.savesBytes ?? 0,
        color: "var(--color-p4)",
      },
    ];
    const total = parts.reduce((a, p) => a + p.bytes, 0);
    const [number, unit] = total ? formatBytes(total).split(" ") : ["0", "MB"];
    const disk = library?.disk;
    return (
      <div className="card dash-card stack-md">
        <div className="dash-card-head">
          <h2 className="card-title">{t.dash.space}</h2>
          <span className="small faint">{t.dash.spaceHint}</span>
        </div>
        <div className="dash-kpi-value">
          <span className="dash-kpi-number">{number}</span>
          <span className="dash-kpi-unit">{unit}</span>
          {disk && disk.total > 0 && (
            <span className="small faint dash-push">
              {t.dash.ofDisk(formatPct((total / disk.total) * 100))}
            </span>
          )}
        </div>
        <div
          className="dash-stack-bar is-track"
          role="img"
          aria-label={parts
            .map((p) => `${p.label} ${formatBytes(p.bytes)}`)
            .join(", ")}
        >
          {total > 0 &&
            parts.map((p) => (
              <span
                key={p.label}
                style={{
                  // A tiny part still shows as a sliver.
                  width: p.bytes
                    ? `max(3px, ${(p.bytes / total) * 100}%)`
                    : "0",
                  background: p.color,
                }}
              />
            ))}
        </div>
        <div className="stack-xs dash-divided">
          {parts.map((p) => (
            <div key={p.label} className="dash-row small">
              <span className="dash-space-label">
                <i style={{ background: p.color }} />
                {p.label}
              </span>
              <span className="mono">
                {formatBytes(p.bytes)}
                <span className="faint">
                  {" · "}
                  {total ? formatPct((p.bytes / total) * 100) : "0%"}
                </span>
              </span>
            </div>
          ))}
        </div>
      </div>
    );
  }
  function libraryCard() {
    const counts: Record<Kind, number> = summary?.kinds ?? { runs: 0, missing: 0, unsupported: 0, broken: 0, bios: 0, unchecked: 0 };
    const parts = (
      [
        "runs",
        "missing",
        "unsupported",
        "broken",
        "bios",
        "unchecked",
      ] as Kind[]
    ).map((kind) => ({ kind, color: KIND_COLOR[kind] }));
    const total = Math.max(romCount, 1);
    return (
      <div className="card dash-card stack-md">
        <div className="dash-card-head">
          <h2 className="card-title">{t.dash.library}</h2>
          <span className="dash-big-number">
            {romCount} <small>{t.dash.sets}</small>
          </span>
        </div>
        <div className="dash-stack-bar">
          {parts.map(
            (p) =>
              counts[p.kind] > 0 && (
                <span
                  key={p.kind}
                  style={{
                    width: `${(counts[p.kind] / total) * 100}%`,
                    background: p.color,
                  }}
                />
              ),
          )}
        </div>
        <dl className="dash-legend-grid">
          {parts
            .filter(
              (p) =>
                counts[p.kind] > 0 || p.kind === "runs" || p.kind === "missing",
            )
            .map((p) => (
              <LegendRow
                key={p.kind}
                color={p.color}
                label={KIND_LABEL[p.kind]()}
                value={String(counts[p.kind])}
              />
            ))}
        </dl>
        {library && (
          <dl className="dash-facts dash-facts-top">
            <dt>{t.dash.core}</dt>
            <dd className={library.core.installed ? "is-ok" : ""}>
              {library.core.installed
                ? `✓ ${library.core.name || "mame2003-plus"}`
                : library.core.downloading
                  ? t.linked.coreDownloading
                  : t.linked.coreMissing}
            </dd>
            <dt>{t.dash.gameList}</dt>
            <dd className={library.core.catalog ? "is-ok" : ""}>
              {library.core.catalog
                ? `✓ ${t.linked.coreInstalled}`
                : library.core.downloading
                  ? t.linked.coreDownloading
                  : t.linked.coreMissing}
            </dd>
          </dl>
        )}
        {library && (!library.core.installed || !library.core.catalog) && (
          <button
            type="button"
            className="button button-secondary"
            disabled={library.core.downloading}
            onClick={() =>
              hostLink?.stream.sendControl({ type: "download_core" })
            }
          >
            {t.linked.coreDownload}
          </button>
        )}
        {library?.core.error && (
          <p className="form-error small">{library.core.error}</p>
        )}
        {library && (
          <Link to="/device/roms" className="dash-link-row">
            {t.dash.browseRoms}
            <span aria-hidden="true">→</span>
          </Link>
        )}
      </div>
    );
  }

  function storageCard() {
    const bytes = romBytes;
    const biggest = summary?.biggest ?? [];
    const disk = library?.disk;
    const used = disk ? disk.total - disk.free : 0;
    const romPct = disk ? (bytes / disk.total) * 100 : 0;
    const otherPct = disk ? (Math.max(used - bytes, 0) / disk.total) * 100 : 0;
    return (
      <div className="card dash-card stack-md">
        <div className="dash-card-head">
          <h2 className="card-title">{t.dash.storage}</h2>
          <span className="small faint">
            {romCount} {t.dash.sets}
          </span>
        </div>
        <div className="dash-kpi-value">
          <span className="dash-kpi-number">
            {bytes ? formatBytes(bytes).split(" ")[0] : "0"}
          </span>
          <span className="dash-kpi-unit">
            {bytes ? formatBytes(bytes).split(" ")[1] : "MB"}
          </span>
          {romCount > 0 && (
            <span className="small faint dash-push">
              {t.dash.avgPerSet(formatBytes(bytes / romCount))}
            </span>
          )}
        </div>
        {disk && (
          <div className="stack-xs">
            <div className="dash-stack-bar is-track">
              <span
                style={{
                  width: `max(3px, ${romPct}%)`,
                  background: "var(--color-accent)",
                }}
              />
              <span
                style={{ width: `${otherPct}%`, background: "var(--color-p4)" }}
              />
            </div>
            <div className="dash-legend-inline">
              <span>
                <i style={{ background: "var(--color-accent)" }} />
                {t.dash.roms}
              </span>
              <span>
                <i style={{ background: "var(--color-p4)" }} />
                {`${t.dash.other} ${formatBytes(Math.max(used - bytes, 0))}`}
              </span>
              <span>
                <i className="is-empty" />
                {`${t.dash.free} ${formatBytes(disk.free)}`}
              </span>
            </div>
          </div>
        )}
        {biggest.length > 0 && (
          <div className="stack-xs dash-divided">
            <span className="dash-mini-title">{t.dash.biggest}</span>
            {biggest.map((r, i) => (
              <div key={r.name} className="stack-xxs">
                <div className="dash-row small">
                  <span className="mono">{r.name}</span>
                  <span className="mono faint">{formatBytes(r.size)}</span>
                </div>
                <div className="dash-bar is-thin">
                  <span
                    style={{
                      width: `${(r.size / biggest[0]!.size) * 100}%`,
                      background:
                        i === 0
                          ? "var(--color-accent)"
                          : "var(--color-accent-dim)",
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        )}
        {disk && (
          <span className="small faint dash-push-bottom">
            {t.dash.diskFree(formatBytes(disk.free), formatBytes(disk.total))}
          </span>
        )}
      </div>
    );
  }
}

function Kpi({
  label,
  color,
  value,
  unit,
  foot,
  children,
}: {
  label: string;
  color: string;
  value: string;
  unit: string;
  foot: string;
  children: ReactNode;
}) {
  return (
    <div className="dash-kpi">
      <div className="dash-kpi-label">
        {label}
        <span style={{ background: color }} />
      </div>
      <div className="dash-kpi-value">
        <span className="dash-kpi-number">{value}</span>
        <span className="dash-kpi-unit">{unit}</span>
      </div>
      <div className="dash-kpi-visual">{children}</div>
      <div className="dash-kpi-foot">{foot}</div>
    </div>
  );
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: "voice" | "accent";
}) {
  return (
    <div className="dash-stat">
      <span className="small muted">{label}</span>
      <span className={`dash-stat-value${tone ? ` is-${tone}` : ""}`}>
        {value}
      </span>
    </div>
  );
}

function LegendRow({
  color,
  label,
  value,
}: {
  color: string;
  label: string;
  value: string;
}) {
  return (
    <div className="dash-legend-row">
      <dt>
        <i style={{ background: color }} />
        {label}
      </dt>
      <dd className="mono">{value}</dd>
    </div>
  );
}

function Seats({
  players,
  max,
  large = false,
}: {
  players: number;
  max: number;
  large?: boolean;
}) {
  return (
    <div className={`dash-seats${large ? " is-large" : ""}`} aria-hidden="true">
      {[1, 2, 3, 4].slice(0, Math.min(Math.max(max, 1), 4)).map((p) => (
        <span
          key={p}
          className={p <= players ? "is-taken" : ""}
          style={{ ["--port-color" as string]: `var(--color-p${p})` }}
        >
          P{p}
        </span>
      ))}
    </div>
  );
}

/**
 * A CPU figure next to the average (one decimal): small values keep their
 * decimal, so a 0.4% peak never reads 0% beside a 0.1% average.
 */
function pct1(v: number): number {
  return v < 10 ? Math.round(v * 10) / 10 : Math.round(v);
}

/** A share as a whole percent; a tiny share that is not zero shows "<1%". */
function formatPct(pct: number): string {
  if (pct > 0 && pct < 1) return "<1%";
  return `${Math.round(pct)}%`;
}

/** /device/history/:roomId/network: a room's network report. */
function NetworkRoute() {
  const { roomId = "" } = useParams();
  return <NetworkReport roomId={roomId} />;
}

