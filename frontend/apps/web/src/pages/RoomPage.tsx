// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
} from "react";
import { createPortal } from "react-dom";
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  DEFAULT_CONTROLS,
  bindingOf,
  groupKeyOf,
  isTouchDevice,
  padMapFor,
  parseInvite,
  recordStart,
  recordStop,
  roomPictureAction,
  savePlayerName,
  savedPlayerName,
  startButtonCount,
  startOf,
} from "@go-link/shared";
import { playDing, setDingOutput } from "../components/ding";
import { t, useLang } from "../i18n";
import { DEMO_DEVICE_NAME } from "../fixtures";
import { useSignal } from "../signal/SignalProvider";
import { setHeaderMenuItems, useHeaderSlot, useMediaQuery } from "../components/headerSlot";
import { useJoinRoom, usePublicRoomMeta } from "../signal/useJoinRoom";
import { useLatencyProbe } from "../signal/useLatencyProbe";
import { useHostStream } from "../signal/useHostStream";
import { AndroidAppCard } from "../components/AndroidAppCard";
import { KnockPrompt } from "../components/KnockPrompt";
import { forgetRoomPass, roomPass, saveRoomPass } from "../signal/roomPasses";
import { ServerHelp } from "../components/ServerSettings";
import {
  ChevronLeftIcon,
  ControllerIcon,
  ExitFullscreenIcon,
  FullscreenIcon,
  HelpIcon,
  GamepadIcon,
  LockIcon,
  MicIcon,
  MicOffIcon,
  PauseIcon,
  HandIcon,
  PictureIcon,
  ChevronRightIcon,
  PlayIcon,
  SoundOffIcon,
  SoundOnIcon,
  SwapIcon,
  ChatIcon,
  UserPlusIcon,
  PowerIcon,
  CameraIcon,
  RecordIcon,
  StopIcon,
  LogOutIcon,
  SlidersIcon,
  CloseIcon,
} from "../components/Icons";
import { Chip, HeroTile, PageHero } from "../components/ui/PageHero";
import { ControlsPanel } from "../components/ControlsPanel";
import {
  ConfirmDialog,
  RemapDialog,
  type RemapTarget,
} from "../components/RemapDialog";
import { useInputConfig } from "../signal/useInputConfig";
import { TouchPad } from "../components/TouchPad";
import { InviteDialog } from "../components/InviteDialog";
import { NameStep } from "../components/NameStep";
import { PauseAskDialog } from "../components/PauseAskDialog";
import { useLinkedPauseAsks } from "../signal/useLinkedPauseAsks";
import { StreamInfo } from "../components/StreamInfo";
import { PlayersCapsule, type SeatSwap } from "../components/PlayersCapsule";
import type { HostTools } from "../components/GuestMenu";
import { TermsCheck } from "../components/legal/TermsCheck";
import { acceptTerms, termsAccepted } from "../legal";
import { HelpDialog } from "../components/HelpDialog";
import { saveScreenshot } from "../components/screenshot";
import { useFullscreen } from "../components/useFullscreen";
import {
  demoModel,
  liveModel,
  type RoomModel, localName } from "./roomModel";
import {
  useAudioLevels,
  useMicrophone,
  useMicTest,
  type MicState,
} from "../signal/useVoice";
import { applySink, useAudioDevices, type AudioDevices } from "../signal/useAudioDevices";
import { AudioDevicesBlock } from "../components/AudioDevices";
import { SidePanel, useUnreadChat, type SideActions, type SideTab } from "./RoomSide";
import { ConsoleDrawer, DRAWER_TABS, type DrawerTab } from "./ConsoleDrawer";
import { inSheet, useSheetLayout } from "../components/sheet";
import { PictureControl } from "../components/PictureControl";
import { PictureCanvas } from "@go-link/ui/picture";
import { SplitDivider } from "../picture/SplitDivider";
import { needsRenderer, usePictureSettings } from "@go-link/ui/picture";
import type { RendererKind } from "@go-link/ui/picture";
import { invitationUrl } from "../role";

const CONTROLS_KEY = "go-link.show-controls";
const TOUCH_KEY = "go-link.touchpad";
const CHAT_HIDDEN_KEY = "go-link.chat-hidden";
const SIDE_TAB_KEY = "go-link.side-tab";
const CHAT_SOUND_KEY = "go-link.chat-sound";
/** The console drawer's last tab, for this browser tab's session. */
const DRAWER_TAB_KEY = "go-link.drawer-tab";

function readDrawerTab(): DrawerTab {
  try {
    const v = sessionStorage.getItem(DRAWER_TAB_KEY) as DrawerTab | null;
    return v && DRAWER_TABS.includes(v) ? v : "chat";
  } catch {
    return "chat";
  }
}
/** The host's "Several controllers" switch, for this browser tab's session. */
const MULTI_PADS_KEY = "go-link.multi-pads";

function readMultiPads(): boolean {
  try {
    return sessionStorage.getItem(MULTI_PADS_KEY) === "true";
  } catch {
    return false;
  }
}
/** How long the controls over the video stay without a movement or tap. */
const IDLE_MS = 3000;

/** play() as a promise, also where it returns nothing (old browsers, tests). */
function tryPlay(media: HTMLMediaElement): Promise<void> {
  return Promise.resolve().then(() => media.play());
}
const KEYBOARD_PLAYER_KEY = "go-link.keyboard-player";
const GAME_VOLUME_KEY = "go-link.game-volume";
const VOICE_VOLUME_KEY = "go-link.voice-volume";

function readVolume(key: string, fallback: number): number {
  const n = Number(readStorage(key));
  return readStorage(key) !== null && Number.isFinite(n) && n >= 0 && n <= 100
    ? n
    : fallback;
}

function readStorage(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeStorage(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    // storage disabled: the choice lasts for this page only
  }
}

function readSession(key: string): string | null {
  try {
    return sessionStorage.getItem(key);
  } catch {
    return null;
  }
}

function writeSession(key: string, value: string): void {
  try {
    sessionStorage.setItem(key, value);
  } catch {
    // storage disabled: the choice lasts for this page only
  }
}

function readKeyboardPlayer(): number {
  const n = Number(readStorage(KEYBOARD_PLAYER_KEY));
  return Number.isInteger(n) && n >= 0 && n < 4 ? n : 0;
}


/**
 * A private room asks for its PIN before the device streams: six digits,
 * checked by the device, which limits the tries.
 */
function PinPrompt({
  busy,
  last,
  onSend,
}: {
  busy: boolean;
  last: { reason: string; left: number; retryAfter: number } | null;
  onSend: (pin: string) => void;
}) {
  const [value, setValue] = useState("");
  // A guest who came straight from a link accepts the terms here.
  const [needsTerms] = useState(() => !termsAccepted());
  const [agreed, setAgreed] = useState(false);
  const [askTerms, setAskTerms] = useState(false);
  const blocked = last?.reason === "blocked";
  const error = !last
    ? ""
    : last.reason === "wrong"
      ? t.room.pinWrong(last.left)
      : last.reason === "used"
        ? t.room.pinUsed
      : last.reason === "locked"
        ? t.room.pinLocked(Math.ceil(last.retryAfter / 60))
        : t.room.pinBlocked;
  return (
    <form
      className="pin-prompt card stack-md"
      onSubmit={(e) => {
        e.preventDefault();
        if (needsTerms && !agreed) {
          setAskTerms(true);
          return;
        }
        if (value.length === 6 && !busy && !blocked) {
          if (needsTerms) acceptTerms();
          onSend(value);
          setValue("");
        }
      }}
    >
      <span className="pin-prompt-icon" aria-hidden="true">
        <LockIcon size={26} />
      </span>
      <div className="stack-xxs">
        <h2 className="card-title">{t.room.pinTitle}</h2>
        <p className="small muted">{t.room.pinText}</p>
      </div>
      <label className="visually-hidden" htmlFor="room-pin">
        {t.room.pinLabel}
      </label>
      <input
        id="room-pin"
        className="input input-mono pin-input"
        inputMode="numeric"
        autoComplete="one-time-code"
        pattern="[0-9]*"
        maxLength={6}
        autoFocus
        disabled={blocked}
        placeholder="••••••"
        aria-invalid={error ? true : undefined}
        value={value}
        onChange={(e) => setValue(e.target.value.replace(/\D/g, "").slice(0, 6))}
      />
      {error && (
        <p className="form-error small" role="alert">
          {error}
        </p>
      )}
      {needsTerms && <TermsCheck guest checked={agreed} onChange={setAgreed} showError={askTerms} />}
      <button
        type="submit"
        className="button button-primary button-block"
        disabled={value.length !== 6 || busy || blocked}
      >
        {busy ? t.room.pinChecking : t.room.pinEnter}
      </button>
    </form>
  );
}

function VolumeSlider({
  label,
  value,
  onChange,
  disabled,
}: {
  label: string;
  value: number;
  onChange: (v: number) => void;
  disabled?: boolean;
}) {
  return (
    <label className="volume">
      <span className="small muted">{label}</span>
      <input
        type="range"
        min={0}
        max={100}
        value={value}
        disabled={disabled}
        aria-label={t.room.volumeLabel(label)}
        onChange={(e) => onChange(Number(e.target.value))}
      />
    </label>
  );
}

interface VolumeProps {
  game: number;
  voice: number;
  onGame: (v: number) => void;
  onVoice: (v: number) => void;
}

/**
 * The voice in the video's dock: the microphone (seated players), a small
 * level meter and a settings popover with the game and voice volumes and
 * what the voice rules are.
 */
function VoiceControl({
  model,
  micOn,
  toggleMic,
  micLevel = 0.6,
  micState = "on",
  volumes,
  devices,
  micTestLevel = null,
  open,
  setOpen,
  spectatorsHear,
}: {
  model: RoomModel;
  micOn: boolean;
  toggleMic: () => void;
  micLevel?: number;
  micState?: MicState;
  volumes?: VolumeProps;
  devices?: AudioDevices;
  micTestLevel?: number | null;
  open: boolean;
  setOpen: (open: boolean) => void;
  /** The host's switch: spectators hear the voices too. */
  spectatorsHear?: { on: boolean; onChange: (on: boolean) => void };
}) {
  const rootRef = useRef<HTMLDivElement>(null);
  const popRef = useRef<HTMLDivElement>(null);
  // Phones get the settings as a sheet from the bottom of the screen.
  const sheet = useSheetLayout();
  // Elsewhere the popover lives inside the video, which clips it: never
  // taller than the video (it scrolls instead).
  const [fitStyle, setFitStyle] = useState<CSSProperties>({});
  useLayoutEffect(() => {
    if (!open || sheet) return;
    const stage = rootRef.current?.closest(".video-stage");
    const pop = popRef.current;
    if (!stage || !pop) return;
    const fit = () => {
      const s = stage.getBoundingClientRect();
      const maxHeight = Math.max(200, s.height - 24);
      pop.style.maxHeight = `${maxHeight}px`;
      pop.style.marginTop = "0px";
      // Then move it back inside the video if it sticks out.
      const p = pop.getBoundingClientRect();
      const shift = p.top < s.top + 12 ? s.top + 12 - p.top : p.bottom > s.bottom - 12 ? s.bottom - 12 - p.bottom : 0;
      setFitStyle({ maxHeight, marginTop: shift });
    };
    fit();
    window.addEventListener("resize", fit);
    return () => window.removeEventListener("resize", fit);
  }, [open, sheet]);
  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!rootRef.current?.contains(target) && !popRef.current?.contains(target)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => (e.key === "Escape" || e.code === "Escape") && setOpen(false);
    document.addEventListener("mousedown", close);
    window.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      window.removeEventListener("keydown", esc);
    };
  }, [open]);
  const player = model.voice === "player";
  const label = micOn ? t.room.micOpen : t.room.micClosed;
  const lit = Math.round(micLevel * 5);
  const title = player
    ? `${t.room.voiceTitle} · ${label}`
    : model.voice === "off"
      ? t.room.voiceOffTitle
      : model.voice === "pending"
        ? t.room.voicePendingTitle
        : t.room.spectatorVoiceTitle;
  const note = player
    ? micState === "denied"
      ? t.room.micDenied
      : micState === "starting"
        ? t.room.micStarting
        : model.hearVoice
          ? t.room.voicePlayersHeard
          : t.room.voicePlayersOnly
    : model.voice === "off"
      ? t.room.voiceOffNote
      : model.voice === "pending"
        ? t.room.voicePendingNote
        : model.hearVoice
          ? t.room.spectatorHearsNote
          : t.room.spectatorVoiceNote;
  return (
    <div className="dock-group voice-control" ref={rootRef}>
      {player ? (
        <button
          type="button"
          className={`icon-button mic-toggle${micOn ? " is-on" : ""}`}
          aria-pressed={micOn}
          aria-label={label}
          data-tip={label}
          onClick={toggleMic}
        >
          {micOn ? <MicIcon size={17} /> : <MicOffIcon size={17} />}
        </button>
      ) : (
        <button
          type="button"
          className="icon-button mic-toggle is-locked"
          disabled
          aria-label={t.room.micUnavailable}
        >
          <LockIcon size={15} />
        </button>
      )}
      {player && (
        <span className="mini-meter" role="img" aria-label={t.room.micLevel}>
          {Array.from({ length: 5 }, (_, i) => (
            <i key={i} className={micOn && i < lit ? "is-lit" : ""} />
          ))}
        </span>
      )}
      <button
        type="button"
        className={`icon-button voice-settings is-mini${open ? " is-on" : ""}`}
        aria-expanded={open}
        aria-label={t.room.voiceSettings}
        data-tip={open ? undefined : t.room.voiceSettings}
        onClick={() => setOpen(!open)}
      >
        <ChevronRightIcon size={12} />
      </button>
      {open && inSheet(sheet, setOpen, (
        <div
          ref={popRef}
          className={`voice-pop${sheet ? " is-sheet" : ""}${devices ? " has-devices" : ""}`}
          role="dialog"
          aria-label={t.room.voiceTitle}
          style={sheet ? undefined : fitStyle}
        >
          {sheet && <span className="sheet-handle" aria-hidden="true" />}
          <span className="strong">{title}</span>
          <span className="small muted">{note}</span>
          {volumes && (
            <div className="stack-sm">
              <VolumeSlider
                label={t.room.gameVolume}
                value={volumes.game}
                onChange={volumes.onGame}
              />
              <VolumeSlider
                label={t.room.voiceVolume}
                value={volumes.voice}
                onChange={volumes.onVoice}
                disabled={model.voice !== "player" && !model.hearVoice}
              />
            </div>
          )}
          {spectatorsHear && (
            <div className="switch-row">
              <div className="stack-xxs">
                <span className="strong">{t.room.spectatorsHear}</span>
                <span className="small muted">{t.room.spectatorsHearDesc}</span>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={spectatorsHear.on}
                aria-label={t.room.spectatorsHear}
                className={`switch${spectatorsHear.on ? " is-on" : ""}`}
                onClick={() => spectatorsHear.onChange(!spectatorsHear.on)}
              >
                <span className="switch-knob" />
              </button>
            </div>
          )}
          {player && !sheet && (
            <span className="small faint">
              {t.room.holdToTalk} <kbd>V</kbd> {t.room.holdToTalkSuffix}
            </span>
          )}
          {devices && (
            <AudioDevicesBlock devices={devices} showMic={player} micLevel={micTestLevel} />
          )}
        </div>
      ))}
    </div>
  );
}

/** Plays one remote voice; the volume is independent from the game. */
function VoiceAudio({
  stream,
  volume,
  muted,
  sinkId,
}: {
  stream: MediaStream;
  volume: number;
  muted: boolean;
  /** The chosen output ("" = the system's default). */
  sinkId: string;
}) {
  const ref = useRef<HTMLAudioElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.srcObject = stream;
  }, [stream]);
  useEffect(() => applySink(ref.current, sinkId), [sinkId]);
  useEffect(() => {
    const a = ref.current;
    if (!a) return;
    a.volume = volume;
    a.muted = muted;
    if (!muted) void a.play().catch(() => undefined);
  }, [volume, muted]);
  return <audio ref={ref} autoPlay hidden />;
}

function RoomProblem({
  title,
  text,
  help,
}: {
  title: string;
  text?: string;
  help?: boolean;
}) {
  return (
    <div className="page">
      <div className="page-body centered">
        <div className="problem stack-md">
          <h1 className="page-title">{title}</h1>
          {text && <p className="muted">{text}</p>}
          {help && <ServerHelp />}
          <Link to="/rooms" className="button button-secondary">
            {t.room.backToLobby}
          </Link>
        </div>
      </div>
    </div>
  );
}

/** The longest an owner's room page waits for its device before joining. */
const OWN_INVITE_WAIT_MS = 8000;

export function RoomPage() {
  // /r/<room_id>, or /g/<invite> (an invitation link or a typed code).
  const { roomId: routeRoomId = "", invite: routeInvite = "" } = useParams();
  const [params] = useSearchParams();
  const { demo, hostLink, savedLink, linkedDevice, sendToDevice, panel } =
    useSignal();
  const location = useLocation();
  // Every room admits only its invitation: the owner's browser, linked to
  // the device, knows the invitation of its games and of the test room.
  const testRoom = demo ? undefined : linkedDevice.status?.room;
  const ownInvite = demo
    ? ""
    : (linkedDevice.status?.rooms.find(
        (r) => r.roomId === routeRoomId && r.invite,
      )?.invite ??
      (testRoom?.room_id === routeRoomId ? (testRoom.invite ?? "") : ""));
  const invite = routeInvite
    ? parseInvite(routeInvite)
    : ownInvite
      ? { invite: ownInvite }
      : null;
  // A guest: a browser with no device of its own.
  const guest = !demo && !hostLink && !savedLink;
  const inputCfg = useInputConfig();
  const [remap, setRemap] = useState<RemapTarget | null>(null);
  const [confirmClose, setConfirmClose] = useState(false);
  const navigate = useNavigate();
  // The gamepad's pause button needs the latest pause state.
  const pauseToggleRef = useRef<() => void>(() => undefined);
  // A browser linked to a device waits for its status before joining on
  // /r/: if the room is its own, only its invitation lets it in. At most a
  // few seconds, in case the device does not answer.
  const [waitedDevice, setWaitedDevice] = useState(false);
  const waitDevice =
    !demo && !routeInvite && (hostLink !== null || savedLink !== null) && linkedDevice.status === null && linkedDevice.state !== "failed" && !waitedDevice;
  useEffect(() => {
    if (!waitDevice) return;
    const id = window.setTimeout(() => setWaitedDevice(true), OWN_INVITE_WAIT_MS);
    return () => window.clearTimeout(id);
  }, [waitDevice]);
  const status = useJoinRoom(routeRoomId, invite, waitDevice);
  // Known from the start on /r, and once joined on /g.
  const roomId = status.kind === "joined" ? status.roomId : routeRoomId;
  const meta = usePublicRoomMeta(roomId);
  const [micOn, setMicOn] = useState(false);
  const joined = !demo && status.kind === "joined" ? status : null;
  const [keyboardPlayer, setKeyboardPlayer] = useState(readKeyboardPlayer);
  const [multiPads, setMultiPadsState] = useState(readMultiPads);
  const setMultiPads = useCallback((on: boolean) => {
    setMultiPadsState(on);
    try {
      sessionStorage.setItem(MULTI_PADS_KEY, String(on));
    } catch {
      // storage disabled: the switch holds while the page lives
    }
  }, []);
  // The last valid name used in this browser (the device checks it too).
  const [playerName, setPlayerName] = useState(savedPlayerName);
  // "What's your name?" comes up once a typed PIN let this browser in.
  const [askName, setAskName] = useState(false);
  const pinSent = useRef(false);
  // The latency test's last reading goes into the room's telemetry too
  // (set below, once the beacon was measured).
  const [probeSeen, setProbeSeen] = useState<{ median: number; last: number } | null>(null);
  const live = useHostStream(
    joined?.hostPeerId ?? null,
    joined?.takeBacklog,
    keyboardPlayer,
    playerName,
    {
      input: inputCfg.input,
      onPauseButton: () => pauseToggleRef.current(),
      suspended: remap !== null,
      multi: multiPads,
      probe: probeSeen,
    },
  );
  const videoRef = useRef<HTMLVideoElement>(null);
  // The latency test: this browser's seat, the local player that holds it
  // and its beacon while the host draws the controllers on the video.
  const myPort = live.room?.you.ports[0];
  const myLocal = myPort ? (live.room?.seats[myPort - 1]?.localPlayer ?? null) : null;
  const beacon = live.room?.inputHud && myPort ? (live.hud[myPort - 1] ?? null) : null;
  const probe = useLatencyProbe(videoRef, beacon, myLocal, live.pressEdges);
  useEffect(() => setProbeSeen(probe ? { median: probe.median, last: probe.last } : null), [probe]);
  const stageRef = useRef<HTMLDivElement>(null);
  // How this browser draws the game (Picture in the dock): the GPU
  // renderer only runs for a style other than the browser's own look.
  // The viewer's own choice wins, then the host's default for the room
  // (room_state.picture), then the site's default.
  const roomPicture = live.room?.picture ?? null;
  const [picture, setPicture, pictureChoice] = usePictureSettings(roomPicture);
  const [pictureOpen, setPictureOpen] = useState(false);
  const lang = useLang();
  const [compare, setCompare] = useState(false);
  const [split, setSplit] = useState(0.5);
  // undefined: not tried yet; null: this browser cannot (plain <video>).
  const [renderer, setRenderer] = useState<RendererKind | null | undefined>(undefined);
  const fullscreen = useFullscreen(stageRef);
  // go-link HD: tell the device how big the picture is on this screen (device
  // pixels, at most 2 per CSS pixel), so it sends the size that fills it; again
  // when the window, the phone's turn or full screen changes it
  const sentWant = useRef<[number, number]>([0, 0]);
  const setVideoWant = live.setVideoWant;
  useEffect(() => {
    const stage = stageRef.current;
    if (!stage || !live.controlOpen) return;
    let timer = 0;
    const measure = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => {
        const r = stage.getBoundingClientRect();
        const v = videoRef.current;
        const aspect = v && v.videoWidth && v.videoHeight ? v.videoWidth / v.videoHeight : 16 / 9;
        const dpr = Math.min(window.devicePixelRatio || 1, 2);
        const w = Math.round(Math.min(r.width, r.height * aspect) * dpr);
        const h = Math.round(w / aspect);
        const [pw, ph] = sentWant.current;
        if (w > 0 && h > 0 && (Math.abs(w - pw) > pw * 0.1 || Math.abs(h - ph) > ph * 0.1)) {
          sentWant.current = [w, h];
          setVideoWant(w, h);
        }
      }, 300);
    };
    sentWant.current = [0, 0];
    measure();
    const ro = typeof ResizeObserver === "function" ? new ResizeObserver(measure) : null;
    ro?.observe(stage);
    window.addEventListener("resize", measure);
    return () => {
      window.clearTimeout(timer);
      ro?.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [live.controlOpen, setVideoWant]);
  const [helpOpen, setHelpOpen] = useState(false);
  // Phones and tablets get the on-screen gamepad instead of the keyboard map.
  const [touch] = useState(() => isTouchDevice());
  const [touchPad, setTouchPad] = useState(
    () => readStorage(TOUCH_KEY) !== "false",
  );
  // Sound is on by default. A browser may refuse to play sound before the
  // person touched the page (autoplay rules): then the video plays muted
  // and the first tap or key anywhere turns the sound on.
  const [soundOn, setSoundOn] = useState(true);
  const [soundBlocked, setSoundBlocked] = useState(false);
  const [showControls, setShowControls] = useState(
    () => readStorage(CONTROLS_KEY) !== "false",
  );
  const [gameVolume, setGameVolume] = useState(() =>
    readVolume(GAME_VOLUME_KEY, 80),
  );
  const [voiceVolume, setVoiceVolume] = useState(() =>
    readVolume(VOICE_VOLUME_KEY, 100),
  );
  // Voices silenced in this browser, by the person's name (a voice track
  // carries whoever is speaking, so a seat or a track is not the person).
  const [silenced, setSilenced] = useState<ReadonlySet<string>>(new Set());
  const [chatHidden, setChatHidden] = useState(
    () => readStorage(CHAT_HIDDEN_KEY) === "true",
  );
  const [chatSound, setChatSound] = useState(
    () => readStorage(CHAT_SOUND_KEY) !== "false",
  );
  const [ptt, setPtt] = useState(false);

  // Voice: the players and the queue talk (the device enforces it too).
  const seated = !demo && (live.room?.you.ports.length ?? 0) > 0;
  const talker = seated || (!demo && (live.room?.you.queuePositions.length ?? 0) > 0);
  const talking = talker && (micOn || ptt);
  // The microphone and output chosen for this browser; the settings open
  // the microphone too, so the person can test it before talking.
  const audio = useAudioDevices();
  const [voiceOpen, setVoiceOpen] = useState(false);
  const mic = useMicrophone(talker && (micOn || ptt || voiceOpen), talking, audio.micId);
  const micTrack = mic.stream?.getAudioTracks()[0] ?? null;
  // A new microphone replaces the track on the same line (replaceTrack).
  useEffect(() => {
    live.setMicTrack(talker ? micTrack : null);
    // setMicTrack is a stable wrapper around the current stream
  }, [talker, micTrack, live.media]);
  // Allowing the microphone reveals the devices' names.
  const { refresh: refreshDevices } = audio;
  useEffect(() => {
    if (mic.stream) refreshDevices();
  }, [mic.stream, refreshDevices]);
  const micTestLevel = useMicTest(micTrack, voiceOpen && seated);
  useEffect(() => setDingOutput(audio.outId), [audio.outId]);

  // The game is the host's: only the host pauses and resumes it (the
  // device refuses anyone else). Seated players ask the host for a pause.
  // P, and a gamepad's pause button, do whichever this browser may.
  const ownRoomEarly = demo
    ? undefined
    : linkedDevice.status?.rooms.find((r) => r.roomId === roomId);
  const roomOwner = live.room?.you.owner === true;
  const hostHere = roomOwner || (hostLink !== null && ownRoomEarly !== undefined);
  const gamePausable = live.room?.pausable === true;
  const pausable = hostHere && gamePausable;
  const paused = live.room?.paused === true;
  const { setPaused: sendPause, requestPause } = live;
  const setPaused = (on: boolean) => {
    if (roomOwner) sendPause(on);
    else if (hostLink && ownRoomEarly)
      sendToDevice({ type: "room_action", id: ownRoomEarly.id, action: on ? "pause" : "resume" });
  };
  const pauseAsked = live.room?.you.pauseAsked ?? null;
  // A seated guest may ask while a game runs, it is not paused and the host
  // can answer.
  const canAskPause = seated && !hostHere && gamePausable && !paused && live.room?.hostOnline === true;
  const askPause = () => {
    if (pauseAsked) requestPause(true);
    else if (canAskPause) requestPause();
  };
  // Pausing a game that is being recorded ends the recording: ask first.
  const recordingRef = useRef(false);
  const [pauseAsk, setPauseAsk] = useState(false);
  // A seat's Start from the players capsule: a short press, like a tap on
  // the arcade's 1P/2P button.
  const pressStart = (port: number) => {
    live.setTouchButtons(startOf(port));
    window.setTimeout(() => live.setTouchButtons(0), 150);
  };
  const togglePause = () => {
    if (!paused && recordingRef.current) setPauseAsk(true);
    else setPaused(!paused);
  };
  pauseToggleRef.current = () => {
    if (pausable) togglePause();
    else if (seated && !hostHere) askPause();
  };
  const pauseKey = bindingOf(inputCfg.input.keyboard, "pause");
  const pauseKeyOn = pausable || (seated && !hostHere);
  useEffect(() => {
    if (!pauseKeyOn || !pauseKey || remap) return;
    const down = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (
        e.code !== pauseKey ||
        e.repeat ||
        (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA"))
      )
        return;
      pauseToggleRef.current();
    };
    window.addEventListener("keydown", down);
    return () => window.removeEventListener("keydown", down);
  }, [pauseKeyOn, pauseKey, remap]);

  // The owner (this browser is linked to the device running this room)
  // can close the game: the room is archived, with its game saved.
  const ownRoom = demo
    ? undefined
    : linkedDevice.status?.rooms.find((r) => r.roomId === roomId);
  const ownsRoom = hostLink !== null && ownRoom !== undefined;
  const recording = live.room?.recording === true || ownRoom?.recording === true;
  recordingRef.current = recording;
  const [shotNote, setShotNote] = useState(false);
  // The device's test pattern room: its owner may invite someone to test.
  const ownTest =
    hostLink !== null && !!testRoom?.room_id && testRoom.room_id === roomId;
  const inviteRoom = ownRoom
    ? { id: ownRoom.id, name: ownRoom.name, invite: ownRoom.invite, inviteCode: ownRoom.inviteCode, ownerKey: ownRoom.ownerKey, groupInvite: ownRoom.groupInvite, watchInvite: ownRoom.watchInvite }
    : ownTest && testRoom
      ? { id: "test", name: testRoom.title ?? "Test pattern", invite: testRoom.invite ?? "", inviteCode: testRoom.invite_code ?? "", ownerKey: testRoom.owner_key ?? "", groupInvite: testRoom.groupInvite, watchInvite: testRoom.watchInvite }
      : null;
  const [inviteOpen, setInviteOpen] = useState(false);
  const linkedAsks = useLinkedPauseAsks();
  // A short notice over the video: the host declined, or only the host pauses.
  const [pauseNote, setPauseNote] = useState("");
  useEffect(() => {
    if (!pauseNote) return;
    const id = window.setTimeout(() => setPauseNote(""), 4000);
    return () => window.clearTimeout(id);
  }, [pauseNote]);
  const lastChat = live.chat[live.chat.length - 1];
  useEffect(() => {
    if (lastChat?.kind === "system" && lastChat.event === "pause_declined") setPauseNote(t.pauseAsk.declined);
  }, [lastChat]);
  useEffect(() => {
    if (live.refused?.code === "pause_owner_only") setPauseNote(t.pauseAsk.ownerOnly);
  }, [live.refused]);
  // A raised hand: the spectator's notices, and the host's sound and notice
  // for each new hand.
  const [handNote, setHandNote] = useState("");
  useEffect(() => {
    if (!handNote) return;
    const id = window.setTimeout(() => setHandNote(""), 4000);
    return () => window.clearTimeout(id);
  }, [handNote]);
  useEffect(() => {
    const code = live.refused?.code;
    if (code === "hand_too_soon") {
      const again = live.room?.you.handAgain ?? 0;
      const s = Math.ceil((again - Date.now()) / 1000);
      setHandNote(s > 0 && s <= 60 ? t.hand.again(s) : t.hand.tooSoon);
    } else if (code === "hand_no_host") setHandNote(t.hand.noHost);
    else if (code === "queue_ask_host") setHandNote(t.hand.askHost);
    // the room state only matters at the moment of the refusal
  }, [live.refused]);
  useEffect(() => {
    if (lastChat?.kind === "system" && lastChat.event === "hand_later") setHandNote(t.hand.later);
  }, [lastChat]);
  const handPeers = useRef(new Set<string>());
  const handsKey = (live.room?.you.owner ? live.room.spectators : [])
    .filter((sp) => sp.hand && sp.peer)
    .map((sp) => `${sp.peer}\u0000${sp.name}`)
    .join("\u0001");
  useEffect(() => {
    const now = new Map(handsKey ? handsKey.split("\u0001").map((x) => x.split("\u0000") as [string, string]) : []);
    const fresh = [...now].filter(([peer]) => !handPeers.current.has(peer));
    handPeers.current = new Set(now.keys());
    const first = fresh[0];
    if (!first) return;
    playDing();
    setHandNote(t.hand.raised(localName(first[1])));
  }, [handsKey]);
  // The host took this browser out of the room: say so, for good.
  const [removed, setRemoved] = useState(false);
  useEffect(() => {
    if (live.pin.last?.reason === "removed") setRemoved(true);
  }, [live.pin.last]);
  const [dockOpen, setDockOpen] = useState(false);
  // The device asks every browser for a PIN, the owner's too (its room
  // page is another connection than the device's link). Before showing the
  // form, try what this browser already has, in order: the host's key, the
  // token this browser got the last time it came in, and the PIN typed on
  // the join page. Only a PIN the person types shows its errors.
  const joinPin = ((location.state as { pin?: unknown } | null)?.pin as string | undefined) ?? "";
  // A group invitation's link carries its key after # (never sent to a
  // server): it lets several people in, each one OK'd by the host when the
  // host asked for that. The guest knocks with a name the host recognises.
  // Read once and kept in memory: the address bar (screenshots, shared
  // screens, the history) no longer shows it once the page has it.
  const [groupKey] = useState(() => (routeInvite ? groupKeyOf(location.hash) : ""));
  useEffect(() => {
    if (groupKey && groupKeyOf(window.location.hash)) {
      window.history.replaceState(window.history.state, "", window.location.pathname + window.location.search);
    }
  }, [groupKey]);
  const credentials = useMemo(() => {
    const list: { token?: string; pin?: string; key?: string; stored?: boolean }[] = [];
    if (inviteRoom?.ownerKey) list.push({ token: inviteRoom.ownerKey });
    const saved = roomId ? roomPass(roomId) : "";
    if (saved) list.push({ token: saved, stored: true });
    if (groupKey) list.push({ key: groupKey });
    if (/^\d{6}$/.test(joinPin)) list.push({ pin: joinPin });
    return list;
  }, [inviteRoom?.ownerKey, roomId, joinPin, groupKey]);
  const autoTry = useRef<{ next: number; sent: (typeof credentials)[number] | null }>({ next: 0, sent: null });
  const [typedPin, setTypedPin] = useState(false);
  // The group invitation: asking for a name, waiting for the host, or closed.
  const [knock, setKnock] = useState<"" | "form" | "sent">("");
  const [knockEnd, setKnockEnd] = useState("");
  const [knockName, setKnockName] = useState(playerName);
  const knocking = useRef(false);
  const sendKnock = useCallback(
    (name: string) => {
      setPlayerName(name);
      savePlayerName(name);
      setKnock("sent");
      knocking.current = true;
      live.sendKey(groupKey, name);
    },
    [live, groupKey],
  );
  useEffect(() => {
    const pin = live.pin;
    if (!pin.needed) {
      knocking.current = false;
      if (pin.token && roomId && !inviteRoom) saveRoomPass(roomId, pin.token);
      // In with a PIN (not a key or a return token): ask for the name.
      if (pin.token && pinSent.current) {
        pinSent.current = false;
        setAskName(true);
      }
      return;
    }
    if (pin.busy) return;
    if (pin.last?.reason === "waiting") return; // the host decides
    // Taken out by the host: the token now answers "removed" (a reload
    // says so again); nothing else is tried.
    if (pin.last?.reason === "removed") return;
    if (pin.last && knocking.current) {
      // The group invitation did not let this person in: say why.
      knocking.current = false;
      setKnockEnd(pin.last.reason || "wrong");
      return;
    }
    if (knock === "form") return; // the person is typing a name
    const tries = autoTry.current;
    if (!pin.last) tries.next = 0; // a new ask: start again
    else if (tries.sent?.stored && roomId) forgetRoomPass(roomId); // an old token
    if (pin.last && tries.sent?.pin) {
      // The PIN the person typed on the join page failed: show why (used,
      // wrong...) and let them type another.
      tries.sent = null;
      setTypedPin(true);
      return;
    }
    if (pin.last && !tries.sent) return; // the person is typing
    const next = credentials[tries.next];
    tries.sent = next ?? null;
    if (!next) return;
    tries.next++;
    if (next.token) live.sendToken(next.token);
    else if (next.key) {
      // A name the host recognises, and the terms the first time.
      if (playerName.trim() && termsAccepted()) sendKnock(playerName.trim());
      else setKnock("form");
    } else if (next.pin) {
      pinSent.current = true;
      live.sendPin(next.pin);
    }
  }, [live, credentials, roomId, inviteRoom, knock, playerName, sendKnock]);
  // Hold V to talk.
  useEffect(() => {
    if (!talker) return;
    const typing = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      return !!el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA");
    };
    const down = (e: KeyboardEvent) =>
      e.code === "KeyV" && !typing(e) && setPtt(true);
    const up = (e: KeyboardEvent) => e.code === "KeyV" && setPtt(false);
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    return () => {
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
    };
  }, [talker]);

  // Your own microphone's level (Web Audio); the others' come from WebRTC.
  const levels = useAudioLevels({ mic: talking ? mic.stream : null }, micOn || ptt);
  // The four voice tracks carry whoever speaks now (room_state.voice_slots):
  // a player lights up their seat, someone in the queue their row.
  const voiceSlots = live.room?.voiceSlots ?? [];
  const speaking = new Set<number>();
  const speakingNames = new Set<string>();
  [1, 2, 3, 4].forEach((n) => {
    const who = voiceSlots[n - 1];
    // WebRTC's level is linear (1 = full scale): speech is well above 0.03.
    if (!who || (live.voiceLevels[n] ?? 0) <= 0.03 || silenced.has(who.name)) return;
    if (who.port) speaking.add(who.port);
    else speakingNames.add(who.name);
  });
  if (talking && (levels.mic ?? 0) > 0.12) {
    live.room?.you.ports.forEach((p) => speaking.add(p));
    if (!seated && live.room) speakingNames.add(live.room.you.name);
  }
  const silencedPorts = new Set(
    (live.room?.seats ?? []).flatMap((s, i) => (s && silenced.has(s.name) ? [i + 1] : [])),
  );

  useEffect(() => {
    if (videoRef.current) videoRef.current.srcObject = live.media;
  }, [live.media]);
  // The game's sound plays on the chosen output (the voices do too).
  useEffect(() => {
    applySink(videoRef.current, audio.outId);
  }, [audio.outId, live.media]);
  useEffect(() => {
    const v = videoRef.current;
    if (!v) return;
    v.muted = !soundOn;
    v.volume = gameVolume / 100;
    if (!soundOn) {
      setSoundBlocked(false);
      return;
    }
    tryPlay(v).then(
      () => setSoundBlocked(false),
      () => {
        v.muted = true;
        void v.play().catch(() => undefined);
        setSoundBlocked(true);
      },
    );
  }, [soundOn, gameVolume, live.media]);
  useEffect(() => {
    if (!soundBlocked) return;
    const unmute = () => {
      const v = videoRef.current;
      if (!v) return;
      v.muted = false;
      tryPlay(v).then(
        () => setSoundBlocked(false),
        () => undefined,
      );
    };
    window.addEventListener("pointerdown", unmute);
    window.addEventListener("keydown", unmute);
    return () => {
      window.removeEventListener("pointerdown", unmute);
      window.removeEventListener("keydown", unmute);
    };
  }, [soundBlocked]);
  const hearing = soundOn && !soundBlocked;

  // Console mode: a phone or tablet with the on-screen gamepad turns the
  // room into a handheld console that fills the screen (a Game Boy held
  // upright, a Switch held sideways). Everything but the game folds into a
  // drawer opened from the side.
  const consoleMode = live.media !== null && touch && touchPad;
  const [drawer, setDrawer] = useState(false);
  const [drawerTab, setDrawerTabState] = useState(readDrawerTab);
  const setDrawerTab = useCallback((tab: DrawerTab) => {
    setDrawerTabState(tab);
    try {
      sessionStorage.setItem(DRAWER_TAB_KEY, tab);
    } catch {
      // storage disabled: the tab is remembered while the page lives
    }
  }, []);
  const closeDrawer = useCallback(() => setDrawer(false), []);
  // Where the room's actions go (read before any early return: hooks).
  const headerInfo = useHeaderSlot("info");
  const headerActions = useHeaderSlot("actions");
  const wide = useMediaQuery("(min-width: 701px)");
  // On wide screens the room's title and actions sit in the main header, one
  // row next to the logo, so the video gets the height.
  const inHeader = wide && headerInfo !== null && headerActions !== null;
  // While a game streams, its Picture settings live in the header's "..."
  // menu (the touch pad keeps them in its dock).
  const pictureInMenu = live.media !== null && !(touch && touchPad);
  useEffect(() => {
    if (!pictureInMenu) return;
    setHeaderMenuItems([{ id: "picture", label: t.picture.button, icon: <PictureIcon />, onSelect: () => {
          setPictureOpen(true);
          wake();
        } }]);
    return () => setHeaderMenuItems([]);
  }, [pictureInMenu, lang]);
  // There the side panel is one canvas with tabs (Chat, Queue, Spectators
  // and, while a game streams, Controls): the chat button opens it on Chat,
  // the dock's controller button on Controls, and the video keeps its height.
  // While a game streams the side panel opens on Controls, so someone new
  // sees the keys at once; the tab chosen last stays for this browser tab.
  const [sideTabWanted, setSideTabState] = useState<SideTab>(() => (readSession(SIDE_TAB_KEY) === "chat" ? "chat" : "controls"));
  const setSideTab = useCallback((tab: SideTab) => {
    setSideTabState(tab);
    writeSession(SIDE_TAB_KEY, tab);
  }, []);
  const controlsTab = inHeader && live.media !== null && !touch;
  const sideTab: SideTab = sideTabWanted === "controls" && !controlsTab ? "chat" : sideTabWanted;
  const showPanel = (open: boolean) => {
    setChatHidden(!open);
    writeStorage(CHAT_HIDDEN_KEY, String(!open));
  };
  /** Opens the side panel on a tab, or closes it if it already shows that tab. */
  const togglePanel = (tab: SideTab) => {
    if (!chatHidden && sideTab === tab) {
      showPanel(false);
      return;
    }
    setSideTab(tab);
    showPanel(true);
  };
  // New lines count as unread while the chat is out of sight: hidden, on
  // another tab of the side panel, or behind a closed drawer or another tab
  // on a phone.
  const chatOutOfSight = consoleMode
    ? !(drawer && drawerTab === "chat")
    : chatHidden || (inHeader && sideTab !== "chat");
  // A new message from someone else: a chime, and a count while unseen.
  const unread = useUnreadChat(
    live.chat,
    live.room?.you.name,
    chatOutOfSight,
    chatSound ? playDing : undefined,
  );
  useEffect(() => {
    if (!consoleMode) {
      setDrawer(false);
      return;
    }
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [consoleMode]);

  // The controls over the video fade out after a few seconds without the
  // mouse moving or a tap, as video players do, and come back on the next
  // movement or tap. A tap on the bare video hides them at once.
  const [idle, setIdle] = useState(false);
  const autoFullscreen = useRef(false);
  const idleTimer = useRef(0);
  const wake = useCallback(() => {
    setIdle(false);
    window.clearTimeout(idleTimer.current);
    const sleep = () => {
      const stage = stageRef.current;
      // Keep them while a menu or a settings panel is open (Picture opens
      // from the header, with no button of its own here) or the mouse rests
      // on them.
      const busy =
        stage?.querySelector('[aria-expanded="true"], .voice-pop') ||
        (window.matchMedia?.("(hover: hover)").matches &&
          stage?.querySelector(".video-toolbar:hover, .players-capsule:hover"));
      if (busy) idleTimer.current = window.setTimeout(sleep, IDLE_MS);
      else setIdle(true);
    };
    idleTimer.current = window.setTimeout(sleep, IDLE_MS);
  }, []);
  useEffect(() => {
    if (live.media !== null) wake();
    else setIdle(false);
    return () => window.clearTimeout(idleTimer.current);
  }, [live.media, wake]);
  const onStagePointer = (e: ReactPointerEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement;
    // Playing on the gamepad is not a request to see the controls.
    if (target.closest(".touchpad-side")) return;
    if (e.type === "pointermove") {
      if (e.pointerType === "mouse") wake();
      return;
    }
    // The first touch of a console hides the browser's bars (where the
    // browser allows it); leaving full screen afterwards is respected.
    if (consoleMode && !autoFullscreen.current) {
      autoFullscreen.current = true;
      fullscreen.enterQuietly();
    }
    const bare = !target.closest("button, a, input, [role='dialog'], .video-toolbar, .players-capsule, .stream-info");
    if (bare && !idle && e.pointerType !== "mouse") {
      window.clearTimeout(idleTimer.current);
      setIdle(true);
    } else wake();
  };

  const toggleFullscreen = () => {
    // Where the page cannot go full screen (iPhone), the stage fills the
    // window instead.
    if (!(consoleMode && !fullscreen.active && fullscreen.enterQuietly()))
      fullscreen.toggle();
  };
  // Phones get the voice settings as a sheet over everything; elsewhere
  // they open from the dock, so the drawer steps aside.
  const sheetLayout = useSheetLayout();
  const openVoiceFromDrawer = () => {
    if (!sheetLayout) {
      setDrawer(false);
      wake();
    }
    setVoiceOpen(true);
  };
  const openPictureFromDrawer = () => {
    if (!sheetLayout) {
      setDrawer(false);
      wake();
    }
    setPictureOpen(true);
  };


  if (!demo) {
    switch (status.kind) {
      case "invalid":
        return (
          <RoomProblem title={t.room.notFoundTitle} text={t.room.invalidId} />
        );
      case "notFound":
        return (
          <RoomProblem
            title={t.room.notFoundTitle}
            text={t.room.notFoundText}
            help
          />
        );
      case "ended":
        return <RoomProblem title={t.room.ended} />;
      case "rateLimited":
        return <RoomProblem title={t.room.rateLimited} />;
      case "unreachable":
        return <RoomProblem title={t.room.unreachable} />;
      default:
        break;
    }
    if (removed) return <RoomProblem title={t.hand.removedTitle} text={t.hand.removedText} />;
  }

  const streaming = live.media !== null;
  const pictureOn = streaming && renderer !== null && (needsRenderer(picture) || compare);
  const touchOn = streaming && touch && touchPad;
  const controlsOn = touch ? touchPad : inHeader ? !chatHidden && sideTab === "controls" : showControls;
  // Demo variants mirror the prototype: ?perspective=spectator&spectatorsHearVoice=true
  const model = demo
    ? demoModel(
        params.get("perspective") !== "spectator",
        micOn,
        params.get("spectatorsHearVoice") === "true",
      )
    : liveModel(live.room, live.chat, { speaking, silenced: silencedPorts, talking, speakingNames });
  // The host (its browser came in with the owner key) moves people toward
  // playing and takes them out of the room; the device checks it is the host.
  const hostTools: HostTools | null =
    !demo && live.room?.you.owner
      ? { seats: model.seats, onPromote: live.promote, onLater: live.handLater, onRemove: live.removeGuest }
      : null;
  // A spectator's raised hand (null: no hand to raise here).
  const spectatorHand = model.me.kind === "spectator" && !live.room?.you.owner ? !!model.me.hand : null;
  const actions: SideActions = demo
    ? { chatEnabled: false, name: "" }
    : {
        chatEnabled: live.controlOpen,
        onChat: live.sendChat,
        onSpectate: live.spectate,
        onQueue: live.joinQueue,
        onHand: live.raiseHand,
        hostTools: hostTools ?? undefined,
        onTyping: live.sendTyping,
        typing: live.typing,
        sound: chatSound,
        onSound: (on) => {
          setChatSound(on);
          writeStorage(CHAT_SOUND_KEY, String(on));
        },
        onHide: () => {
          setChatHidden(true);
          writeStorage(CHAT_HIDDEN_KEY, "true");
        },
        chatOff: live.room ? !live.room.chat : false,
        onChatSwitch:
          ownsRoom && ownRoom
            ? (on) =>
                sendToDevice({
                  type: "room_action",
                  id: ownRoom.id,
                  action: on ? "chat_on" : "chat_off",
                })
            : undefined,
        name: playerName,
        onName: (name) => {
          setPlayerName(name);
          savePlayerName(name);
        },
      };
  const info = live.room?.info;
  const title = demo
    ? "Laundry co-op"
    : meta?.title || info?.title || t.room.private;
  const isPrivate = demo || meta === null;
  const subtitle = demo
    ? `Robo Laundry Blitz · ${t.room.hostLabel(DEMO_DEVICE_NAME)}`
    : [
        meta?.game || info?.game,
        (meta?.host || info?.host) &&
          t.room.hostLabel(meta?.host || info?.host || ""),
      ]
        .filter(Boolean)
        .join(" · ");
  // The link of this page: /g/<invite> for invitations, /r/<id> otherwise.
  const invitePath = /^\/g\/([^/]+)$/.exec(window.location.pathname);
  const inviteUrl = invitePath?.[1] ? invitationUrl(invitePath[1], panel) : `${window.location.origin}${window.location.pathname}`;
  const joining = !demo && status.kind === "joining";
  // The PIN form shows only once what this browser already has (the
  // owner's key, a return token, the join page's PIN) was tried, or for a
  // PIN the person types: the owner never sees it flash while getting in.
  // The name form stays while it is filled in, even if the connection to
  // the device is set up again meanwhile (that asks for the PIN again).
  const knockMode: "form" | "waiting" | "ended" | null = knockEnd
    ? "ended"
    : knock === "form"
      ? "form"
      : live.pin.needed && (live.pin.last?.reason === "waiting" || (knock === "sent" && live.pin.busy))
        ? "waiting"
        : null;
  const pinForm = !knockMode && live.pin.needed && (typedPin || (!live.pin.busy && autoTry.current.next >= credentials.length));
  const playing = model.me.kind === "player";
  const myPorts = demo ? [] : (live.room?.you.ports ?? []);
  const seatedCount = live.room?.seats.filter(Boolean).length ?? 0;
  const starts = startButtonCount(
    live.room?.controls.players ?? 0,
    seatedCount,
  );
  const swapFrom = myPorts[0] ?? 0;
  const swapFor = (port: number): SeatSwap | undefined =>
    !swapFrom || port === swapFrom
      ? undefined
      : {
          onSwap: () => live.swapSeat(swapFrom, port),
          waiting: !!live.room?.you.swapAsked.some(
            (a) => a.from === swapFrom && a.to === port,
          ),
        };
  const swapOffers = demo ? [] : (live.room?.you.swapOffers ?? []);
  // Requests for a pause, for the host: in the room (it came in with the
  // owner key) or through the linked device.
  const hostAsks = demo
    ? []
    : roomOwner
      ? (live.room?.you.pauseAsks ?? []).map((ask) => ({ ask, answer: (accept: boolean) => live.answerPause(ask.from, accept) }))
      : linkedAsks.asks
          .filter((a) => a.roomId === roomId)
          .map((a) => ({ ask: a.ask, answer: (accept: boolean) => linkedAsks.answer(a.id, a.ask.from, accept) }));
  const toggleSilence = (port: number) => {
    const name = live.room?.seats[port - 1]?.name;
    if (!name) return;
    setSilenced((cur) => {
      const next = new Set(cur);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const controlsPanel = (
    <ControlsPanel
      controllers={live.controllers}
      heldKeys={live.heldKeys}
      keyboardPlayer={keyboardPlayer}
      onKeyboardPlayer={(player) => {
        setKeyboardPlayer(player);
        writeStorage(KEYBOARD_PLAYER_KEY, String(player));
      }}
      input={inputCfg.input}
      onRemap={setRemap}
      onPlayer={inputCfg.setPlayer}
      game={live.room?.controls}
      single={live.single}
      onMulti={live.room?.you.owner ? setMultiPads : undefined}
      latency={{
        rttMs: live.stats.rttMs,
        videoMs:
          live.stats.videoBufferMs != null
            ? live.stats.videoBufferMs + (live.stats.decodeMs ?? 0)
            : null,
        hudOn: live.room?.inputHud ?? false,
        seated: myPort !== undefined,
        probe,
        // The host's switch, in game rooms and in the test pattern room.
        onHud:
          ownsRoom || ownTest
            ? (on) =>
                sendToDevice({
                  type: "room_action",
                  id: ownRoom ? ownRoom.id : "test",
                  action: on ? "input_hud_on" : "input_hud_off",
                })
            : undefined,
      }}
    />
  );
  const chatShown = !chatHidden && !(inHeader && sideTab !== "chat");
  const roomActions = (
    <>
      {!demo && (
        <button
          type="button"
          className={`icon-button tip-below icon-with-badge${chatShown ? " is-on" : ""}`}
          aria-label={chatShown ? t.room.chatHide : t.room.chatShow}
          data-tip={chatShown ? t.room.chatHide : t.room.chatShow}
          aria-pressed={chatShown}
          onClick={() => (inHeader ? togglePanel("chat") : showPanel(chatHidden))}
        >
          <ChatIcon />
          {!chatShown && unread > 0 && (
            <span className="icon-badge">{unread > 99 ? "99+" : unread}</span>
          )}
        </button>
      )}
      <button
        type="button"
        className="icon-button tip-below"
        onClick={() => setHelpOpen(true)}
        aria-label={t.help.button}
        data-tip={t.help.button}
      >
        <HelpIcon />
      </button>
      {inviteRoom && (
        <button
          type="button"
          className="icon-button icon-button-primary tip-below"
          aria-label={t.invite.button}
          data-tip={t.invite.button}
          onClick={() => setInviteOpen(true)}
        >
          <UserPlusIcon />
        </button>
      )}
      {streaming && ownsRoom && ownRoom && live.room?.pausable && (
        <button
          type="button"
          className={`icon-button tip-below video-record${recording ? " is-recording" : ""}`}
          aria-pressed={recording}
          aria-label={recording ? t.rec.stop : t.rec.start}
          data-tip={recording ? t.rec.stop : paused ? t.rec.pauseText : `${t.rec.start} · ${t.rec.limits}`}
          disabled={!recording && paused}
          onClick={() => sendToDevice(recording ? recordStop(ownRoom.id) : recordStart(ownRoom.id))}
        >
          {recording ? <StopIcon size={16} /> : <RecordIcon size={16} />}
        </button>
      )}
      {ownsRoom && (
        <button
          type="button"
          className="icon-button icon-button-danger tip-below"
          aria-label={t.room.closeGame}
          data-tip={t.room.closeGame}
          onClick={() => setConfirmClose(true)}
        >
          <PowerIcon />
        </button>
      )}
      <Link
        to="/rooms"
        className="icon-button tip-below"
        aria-label={t.room.leave}
        data-tip={t.room.leaveHint}
      >
        <LogOutIcon />
      </Link>
    </>
  );

  const hero = (
    <PageHero
      className={`room-hero room-bar${inHeader ? " is-in-header" : ""}`}
      tile={
        <HeroTile
          art={meta?.art ?? info?.art}
          status={streaming ? "live" : live.state === "failed" ? "failed" : "idle"}
        >
          <GamepadIcon size={32} />
        </HeroTile>
      }
      // In the header only the title and the players count: the game and host
      // are the title's tooltip, and Leave goes back to the rooms.
      eyebrow={
        inHeader ? undefined : <>
          <Link to="/rooms" className="back-link">
            <ChevronLeftIcon size={14} />
            {t.nav.backShort}
          </Link>
          <span aria-hidden="true">·</span>
          {isPrivate && <LockIcon size={12} />}
          {isPrivate ? t.room.private : t.room.public}
        </>
      }
      title={<span className="room-title" title={inHeader ? subtitle || undefined : undefined}>{title}</span>}
      chips={
        <>
          {subtitle && !inHeader && <Chip>{subtitle}</Chip>}
          {!demo && live.room && (
            <Chip className="chip-keep">
              {t.room.playersChip(seatedCount, live.room.maxPlayers)}
            </Chip>
          )}
          {ownsRoom && !inHeader && <Chip mono>{t.room.roomShort(roomId)}</Chip>}
          {guest && (
            <Chip className="chip-guest chip-keep" title={t.guest.badgeHint}>
              {t.guest.badge}
            </Chip>
          )}
        </>
      }
      actions={inHeader ? undefined : roomActions}
    />
  );

  return (
    <div className={`page room-page${consoleMode ? " is-console-mode" : ""}${inHeader ? " is-header-hero" : ""}`}>
      {inHeader ? (
        <>
          {createPortal(hero, headerInfo!)}
          {createPortal(roomActions, headerActions!)}
        </>
      ) : (
        hero
      )}

      <div className="page-body room-layout">
        <div className="room-main">
          <div
            ref={stageRef}
            className={`video-stage${touchOn ? " has-touchpad is-console" : ""}${!streaming && pinForm ? " needs-pin" : ""}${fullscreen.pseudo ? " is-pseudo-fullscreen" : ""}${fullscreen.active ? " is-fullscreen" : ""}${idle && streaming && !paused ? " is-idle" : ""}`}
            style={
              { "--ar": String(live.aspect ?? 4 / 3) } as CSSProperties
            }
            onPointerDown={onStagePointer}
            onPointerMove={onStagePointer}
            data-picture={pictureOn ? (renderer ?? "pending") : "off"}
            data-picture-style={picture.style}
            data-picture-bands={picture.bands}
          >
            <video
              ref={videoRef}
              className={`video${live.aspect ? " has-aspect" : ""}`}
              style={
                live.aspect ? { aspectRatio: String(live.aspect) } : undefined
              }
              autoPlay
              playsInline
              muted
              hidden={!streaming}
            />
            {pictureOn && (
              <div className="picture-layer">
                <PictureCanvas
                  source={videoRef}
                  aspect={live.aspect ?? 4 / 3}
                  style={picture.style}
                  bands={picture.bands}
                  split={compare ? split : null}
                  native={live.video?.scale === 2 ? { w: live.video.width, h: live.video.height } : null}
                  onRenderer={setRenderer}
                />
                {compare && (
                  <SplitDivider value={split} onChange={setSplit} after={t.picture.styles[picture.style]} />
                )}
              </div>
            )}
            {shotNote && (
              <p className="video-chip shot-note" role="status">
                <CameraIcon size={14} />
                {t.rec.screenshotSaved}
              </p>
            )}
            {streaming && soundBlocked && (
              <p className="video-chip sound-hint" role="status">
                <SoundOffIcon size={14} />
                {t.room.tapForSound}
              </p>
            )}
            {audio.notice && (
              <p className="video-chip audio-toast" role="status">
                {audio.notice.kind === "in" ? <MicIcon size={16} /> : <SoundOnIcon size={16} />}
                {audio.notice.kind === "in"
                  ? t.audio.micGone(audio.notice.name || t.audio.microphone)
                  : t.audio.outGone(audio.notice.name || t.audio.output)}
              </p>
            )}
            {!streaming && knockMode && (
              <div className="pin-stack">
                <KnockPrompt
                  mode={knockMode}
                  reason={knockEnd}
                  name={knockName}
                  onName={setKnockName}
                  onSend={sendKnock}
                  onLeave={() => navigate(guest ? "/" : "/rooms")}
                />
              </div>
            )}
            {!streaming && pinForm && (
              <div className="pin-stack">
              <PinPrompt
                busy={live.pin.busy}
                last={typedPin ? live.pin.last : null}
                onSend={(pin) => {
                  autoTry.current.sent = null;
                  setTypedPin(true);
                  pinSent.current = true;
                  live.sendPin(pin);
                }}
              />
              {routeInvite && <AndroidAppCard />}
              </div>
            )}
            {!streaming && !pinForm && !knockMode && (
              <div className="video-placeholder">
                <GamepadIcon size={40} />
                <span className="small-plus">
                  {joining || live.pin.needed
                    ? t.room.joining
                    : live.state === "failed"
                      ? t.room.videoFailed
                      : demo
                        ? t.room.videoPlaceholder
                        : t.room.waitingVideo}
                </span>
              </div>
            )}
            {(demo || streaming) && (
              <>
                <span className="video-chip video-live">
                  <span className="dot dot-accent dot-small" />
                  {t.room.live}
                </span>
                {recording && (
                  <RecChip
                    since={ownsRoom ? ownRoom?.recordingSince : undefined}
                  />
                )}
                <StreamInfo
                  rttMs={demo ? 38 : live.stats.rttMs}
                  sentFps={demo ? 60 : live.sentFps}
                  receivedFps={demo ? 60 : live.stats.fps}
                  path={demo ? "direct" : live.stats.path}
                  codec={demo ? "VP8" : live.stats.codec}
                  video={demo ? null : live.video}
                  picture={
                    pictureOn && renderer
                      ? `${t.picture.styles[picture.style]} · ${renderer === "webgl2" ? "WebGL 2" : "WebGL 1"}`
                      : t.picture.rendererOff
                  }
                />
              </>
            )}
            {touchOn && (
              <TouchPad
                controls={live.room?.controls ?? DEFAULT_CONTROLS}
                onChange={live.setTouchButtons}
                starts={starts}
                myPorts={myPorts}
              />
            )}
            {streaming && paused && (
              <div className="video-paused" role="status">
                <span className="video-paused-label">
                  {t.room.pausedBy(localName(live.room?.pausedBy ?? ""))}
                </span>
                {pausable && (
                  <button
                    type="button"
                    className="button button-primary button-small"
                    onClick={() => setPaused(false)}
                  >
                    <PlayIcon size={16} /> {t.room.resumeButton}
                  </button>
                )}
              </div>
            )}
            {!streaming && (
              <span className="video-chip video-controls">
                {playing ? t.room.controlsPlayer : t.room.controlsSpectator}
              </span>
            )}
            {streaming &&
              !touchOn &&
              !playing &&
              model.me.kind !== "unknown" && (
                <span className="video-chip video-controls">
                  {t.room.controlsSpectator}
                </span>
              )}
            {(demo || streaming || live.room) && (
              <PlayersCapsule
                seats={model.seats}
                swapFor={(port) => (demo ? undefined : swapFor(port))}
                onToggleSilence={demo ? undefined : toggleSilence}
                onStart={streaming && playing && !touchOn ? pressStart : undefined}
                onRelease={!demo && streaming && (ownsRoom || ownTest) ? live.releaseSeat : undefined}
                queueWaiting={model.queue.length > 0}
                hands={model.hands}
                hostTools={streaming ? (hostTools ?? undefined) : undefined}
              />
            )}
            {streaming && pauseAsked && (
              <div className="swap-offer swap-toast pause-asked" role="status">
                <span className="dot dot-accent" aria-hidden="true" />
                <span className="grow">{t.pauseAsk.asked}</span>
                <button
                  type="button"
                  className="button button-secondary button-small"
                  aria-label={t.pauseAsk.cancelAsk}
                  onClick={() => requestPause(true)}
                >
                  {t.pauseAsk.cancel}
                </button>
              </div>
            )}
            {streaming && pauseNote && !pauseAsked && (
              <p className="video-chip pause-note" role="status">
                <PauseIcon size={14} />
                {pauseNote}
              </p>
            )}
            {streaming && handNote && !(pauseNote && !pauseAsked) && (
              <p className="video-chip pause-note hand-note" role="status">
                <HandIcon size={14} />
                {handNote}
              </p>
            )}
            {hostAsks.map(({ ask, answer }) => (
              <PauseAskDialog
                key={`${ask.from}-${ask.expiresAt}`}
                ask={ask}
                className="pause-ask-stage"
                onAnswer={answer}
              />
            ))}
            {askName && !demo && (
              <div className="name-overlay">
                <NameStep
                  initial={playerName}
                  onDone={(name) => {
                    setPlayerName(name);
                    savePlayerName(name);
                    setAskName(false);
                  }}
                />
              </div>
            )}
            {swapOffers.map((o) => (
              <div
                key={`${o.from}-${o.to}`}
                className="swap-offer swap-toast"
                role="alertdialog"
                aria-label={t.room.swapOffer(localName(o.name), o.from, o.to)}
              >
                <SwapIcon />
                <span className="grow">
                  {t.room.swapOffer(localName(o.name), o.from, o.to)}
                </span>
                <button
                  type="button"
                  className="button button-primary button-small"
                  onClick={() => live.answerSwap(o.from, o.to, true)}
                >
                  {t.room.swapYes}
                </button>
                <button
                  type="button"
                  className="button button-secondary button-small"
                  onClick={() => live.answerSwap(o.from, o.to, false)}
                >
                  {t.room.swapNo}
                </button>
              </div>
            ))}
            {consoleMode && (
              <>
                <span className="console-title">{title}</span>
                <button
                  type="button"
                  className="console-tab"
                  aria-expanded={drawer}
                  aria-label={t.room.consoleMenu}
                  onClick={() => {
                    // Unread messages open the chat; else the last tab.
                    if (unread > 0) setDrawerTab("chat");
                    setDrawer(true);
                  }}
                >
                  <ChatIcon size={16} />
                  {unread > 0 && <span className="toggle-badge">{unread}</span>}
                </button>
              </>
            )}
            {/* Phones held sideways: the dock folds behind one button. */}
            <button
              type="button"
              className={`icon-button dock-toggle${dockOpen ? " is-on" : ""}`}
              aria-expanded={dockOpen}
              aria-label={dockOpen ? t.room.dockClose : t.room.dockOpen}
              onClick={() => setDockOpen(!dockOpen)}
            >
              {dockOpen ? <CloseIcon size={16} /> : <SlidersIcon />}
            </button>
            <div className={`video-toolbar${dockOpen ? " is-open" : ""}`}>
              {!demo && streaming && spectatorHand !== null && (
                <button
                  type="button"
                  className={`icon-button video-hand${spectatorHand ? " is-on" : ""}`}
                  aria-label={spectatorHand ? t.hand.lower : t.hand.raise}
                  aria-pressed={spectatorHand}
                  data-tip={spectatorHand ? t.hand.lower : t.hand.raise}
                  onClick={() => {
                    if (!spectatorHand && live.room?.hostOnline !== true) setHandNote(t.hand.noHost);
                    else live.raiseHand(!spectatorHand);
                  }}
                >
                  <HandIcon />
                </button>
              )}
              <button
                type="button"
                className={`icon-button video-fullscreen${fullscreen.active ? " is-on" : ""}`}
                aria-label={
                  fullscreen.active ? t.touch.exitFullscreen : t.room.fullscreen
                }
                aria-pressed={fullscreen.active}
                data-tip={
                  fullscreen.active ? t.touch.exitFullscreen : t.room.fullscreen
                }
                onClick={toggleFullscreen}
              >
                {fullscreen.active ? (
                  <ExitFullscreenIcon />
                ) : (
                  <FullscreenIcon />
                )}
              </button>
              {(demo || streaming) && (
                <VoiceControl
                  model={model}
                  micOn={demo ? micOn : micOn || ptt}
                  micLevel={demo ? 0.6 : (levels.mic ?? 0)}
                  micState={demo ? "on" : mic.state}
                  devices={audio}
                  micTestLevel={demo ? 0.5 : micTestLevel}
                  open={voiceOpen}
                  setOpen={setVoiceOpen}
                  toggleMic={() => {
                    if (!demo && !micOn) setSoundOn(true); // talking implies listening
                    setMicOn(!micOn);
                  }}
                  volumes={
                    demo
                      ? undefined
                      : {
                          game: gameVolume,
                          voice: voiceVolume,
                          onGame: (v) => {
                            setGameVolume(v);
                            writeStorage(GAME_VOLUME_KEY, String(v));
                          },
                          onVoice: (v) => {
                            setVoiceVolume(v);
                            writeStorage(VOICE_VOLUME_KEY, String(v));
                          },
                        }
                  }
                  spectatorsHear={
                    !demo && live.room && (ownsRoom || ownTest)
                      ? {
                          on: live.room.spectatorsHear,
                          onChange: (on) =>
                            sendToDevice({
                              type: "room_action",
                              id: ownRoom ? ownRoom.id : "test",
                              action: on ? "spectators_voice_on" : "spectators_voice_off",
                            }),
                        }
                      : undefined
                  }
                />
              )}
              {streaming && pausable && (
                <button
                  type="button"
                  className={`icon-button video-pause${paused ? " is-on" : ""}`}
                  aria-pressed={paused}
                  aria-label={paused ? t.room.resume : t.room.pause}
                  data-tip={paused ? t.room.resume : t.room.pause}
                  onClick={togglePause}
                >
                  {paused ? <PlayIcon /> : <PauseIcon />}
                </button>
              )}
              {streaming && seated && !hostHere && !paused && (
                <button
                  type="button"
                  className={`icon-button video-pause video-pause-ask${pauseAsked ? " is-on" : ""}`}
                  aria-pressed={!!pauseAsked}
                  aria-disabled={!pauseAsked && !canAskPause ? true : undefined}
                  aria-label={pauseAsked ? t.pauseAsk.cancelAsk : t.pauseAsk.ask}
                  data-tip={
                    pauseAsked
                      ? t.pauseAsk.cancelAsk
                      : !gamePausable
                        ? t.pauseAsk.notPausable
                        : live.room?.hostOnline !== true
                          ? t.pauseAsk.hostOffline
                          : `${t.pauseAsk.ask} (P)`
                  }
                  onClick={askPause}
                >
                  {pauseAsked ? <CloseIcon /> : <PauseIcon />}
                </button>
              )}
              {streaming && (
                <button
                  type="button"
                  className={`icon-button video-controls-toggle${controlsOn ? " is-on" : ""}`}
                  aria-pressed={controlsOn}
                  aria-label={
                    touch
                      ? controlsOn
                        ? t.touch.hide
                        : t.touch.show
                      : controlsOn
                        ? t.controls.hide
                        : t.controls.show
                  }
                  data-tip={
                    touch
                      ? undefined
                      : live.controllers.length
                        ? live.controllers
                            .map((c) => t.room.controller(c.player, c.name))
                            .join(" · ")
                        : t.controls.keyboardTitle
                  }
                  onClick={() =>
                    touch
                      ? setTouchPad((v) => {
                          writeStorage(TOUCH_KEY, String(!v));
                          return !v;
                        })
                      : inHeader
                        ? togglePanel("controls")
                        : setShowControls((v) => {
                            writeStorage(CONTROLS_KEY, String(!v));
                            return !v;
                          })
                  }
                >
                  <ControllerIcon />
                  {live.controllers.length > 0 && (
                    <span className="toggle-badge">
                      {live.controllers.length}
                    </span>
                  )}
                </button>
              )}
              {streaming && (
                <button
                  type="button"
                  className={`icon-button video-sound${hearing ? "" : " is-muted"}`}
                  aria-pressed={hearing}
                  aria-label={hearing ? t.room.soundOff : t.room.soundOn}
                  data-tip={hearing ? t.room.soundOff : t.room.soundOn}
                  // While the browser blocks sound, this tap itself unmutes it.
                  onClick={() => (soundBlocked ? undefined : setSoundOn(!soundOn))}
                >
                  {hearing ? <SoundOnIcon /> : <SoundOffIcon />}
                </button>
              )}
              {streaming && (
                <PictureControl
                  hideButton={!touchOn}
                  open={pictureOpen}
                  setOpen={setPictureOpen}
                  settings={picture}
                  onChange={setPicture}
                  compare={compare}
                  onCompare={setCompare}
                  available={renderer !== null}
                  roomDefault={roomPicture}
                  saved={pictureChoice.saved}
                  onReset={pictureChoice.reset}
                  onSetRoomDefault={
                    inviteRoom
                      ? () => sendToDevice(roomPictureAction(inviteRoom.id, picture))
                      : undefined
                  }
                />
              )}
              {streaming && (
                <button
                  type="button"
                  className="icon-button video-shot"
                  aria-label={t.rec.screenshot}
                  data-tip={t.rec.screenshot}
                  onClick={() => {
                    const v = videoRef.current;
                    if (v && saveScreenshot(v, title, live.aspect ?? null)) {
                      setShotNote(true);
                      window.setTimeout(() => setShotNote(false), 2500);
                    }
                  }}
                >
                  <CameraIcon />
                </button>
              )}


            </div>
          </div>

          {streaming && !touch && showControls && !inHeader && controlsPanel}
          {remap && (
            <RemapDialog
              target={remap}
              keyboard={inputCfg.input.keyboard}
              pad={
                remap.kind === "pad" ? padMapFor(inputCfg.input, remap.id) : {}
              }
              onKeyboard={inputCfg.setKeyboard}
              onPad={(map) =>
                remap.kind === "pad" && inputCfg.setPad(remap.id, map)
              }
              onReset={() =>
                remap.kind === "pad"
                  ? inputCfg.resetPad(remap.id)
                  : inputCfg.resetKeyboard()
              }
              onClose={() => setRemap(null)}
            />
          )}
          {inviteOpen && inviteRoom && (
            <InviteDialog
              room={inviteRoom}
              fallbackUrl={inviteUrl}
              onAction={(action) =>
                sendToDevice({ type: "room_action", id: inviteRoom.id, action })
              }
              onClose={() => setInviteOpen(false)}
              saver={
                live.room
                  ? {
                      on: live.room.spectatorSaver,
                      onChange: (on) =>
                        sendToDevice({ type: "room_action", id: inviteRoom.id, action: on ? "spectator_saver_on" : "spectator_saver_off" }),
                    }
                  : undefined
              }
            />
          )}
          {helpOpen && <HelpDialog onClose={() => setHelpOpen(false)} />}
          {pauseAsk && (
            <ConfirmDialog
              title={t.rec.pauseTitle}
              text={t.rec.pauseText}
              confirm={t.rec.pauseConfirm}
              onCancel={() => setPauseAsk(false)}
              onConfirm={() => {
                setPauseAsk(false);
                setPaused(true);
              }}
            />
          )}
          {confirmClose && (
            <ConfirmDialog
              title={t.room.closeTitle}
              text={ownRoom?.noSaves ? t.room.closeTextNoSaves : t.room.closeText}
              confirm={t.room.closeConfirm}
              alternative={
                ownRoom?.noSaves && ownRoom.state === "live"
                  ? {
                      label: t.gameRooms.pauseInstead,
                      onClick: () => {
                        setConfirmClose(false);
                        sendToDevice({ type: "room_action", id: ownRoom.id, action: "pause" });
                      },
                    }
                  : undefined
              }
              onCancel={() => setConfirmClose(false)}
              onConfirm={() => {
                setConfirmClose(false);
                if (ownRoom) {
                  sendToDevice({
                    type: "room_action",
                    id: ownRoom.id,
                    action: "archive",
                  });
                  // The room is gone for good: back to the rooms, not to a
                  // "room not found" page.
                  navigate("/rooms", { replace: true });
                }
              }}
            />
          )}


          {/* Voice of the other players, each with its own volume. */}
          {Object.entries(live.voiceStreams).map(([slot, stream]) => (
            <VoiceAudio
              key={slot}
              stream={stream}
              volume={voiceVolume / 100}
              muted={!soundOn || silenced.has(voiceSlots[Number(slot) - 1]?.name ?? "\u0000")}
              sinkId={audio.outId}
            />
          ))}

        </div>

        {consoleMode ? (
          <>
            {drawer && (
              <button
                type="button"
                className="console-scrim"
                aria-label={t.room.consoleClose}
                onClick={() => setDrawer(false)}
              />
            )}
            <ConsoleDrawer
              open={drawer}
              onClose={closeDrawer}
              tab={drawerTab}
              onTab={setDrawerTab}
              unread={unread}
              model={model}
              actions={actions}
              swapFor={(port) => (demo ? undefined : swapFor(port))}
              onToggleSilence={demo ? undefined : toggleSilence}
              swapOffers={swapOffers}
              onAnswerSwap={live.answerSwap}
              fullscreen={fullscreen.active}
              onFullscreen={toggleFullscreen}
              onVoice={openVoiceFromDrawer}
              onPicture={openPictureFromDrawer}
              onHelp={() => setHelpOpen(true)}
              onInvite={
                inviteRoom
                  ? () => {
                      setDrawer(false);
                      setInviteOpen(true);
                    }
                  : undefined
              }
              onCloseGame={
                ownsRoom
                  ? () => {
                      setDrawer(false);
                      setConfirmClose(true);
                    }
                  : undefined
              }
            />
          </>
        ) : (
          !(chatHidden && !demo) && (
            inHeader ? (
              <SidePanel
                model={model}
                actions={actions}
                tab={sideTab}
                onTab={setSideTab}
                controls={controlsTab ? controlsPanel : undefined}
                onClose={() => showPanel(false)}
              />
            ) : (
              <SidePanel model={model} actions={actions} />
            )
          )
        )}
      </div>
    </div>
  );
}

/** REC over the video; the owner also sees for how long. */
function RecChip({ since }: { since?: string }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!since) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [since]);
  const start = since ? new Date(since).getTime() : NaN;
  const s = Number.isFinite(start) ? Math.max(0, Math.floor((now - start) / 1000)) : -1;
  const p = (n: number) => String(n).padStart(2, "0");
  const clock = s < 0 ? "" : s >= 3600 ? `${Math.floor(s / 3600)}:${p(Math.floor(s / 60) % 60)}:${p(s % 60)}` : `${p(Math.floor(s / 60))}:${p(s % 60)}`;
  return (
    <span className="video-chip video-rec" role="status" title={t.rec.everyoneHint} aria-label={t.rec.everyone}>
      <i aria-hidden="true" />
      {t.rec.badge}
      {clock && <span className="mono">{clock}</span>}
    </span>
  );
}
