// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { useEffect, useRef, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { Link } from "react-router-dom";
import { t } from "../i18n";
import {
  BellIcon,
  BellOffIcon,
  ChatIcon,
  CloseIcon,
  ExitFullscreenIcon,
  FullscreenIcon,
  HelpIcon,
  LogOutIcon,
  PowerIcon,
  SlidersIcon,
  PictureIcon,
  SoundOffIcon,
  SoundOnIcon,
  SwapIcon,
  UserPlusIcon,
} from "../components/Icons";
import { portStyle } from "../components/Seats";
import type { SeatSwap } from "../components/PlayersCapsule";
import { localName, type RoomModel } from "./roomModel";
import {
  ChatForm,
  ChatLog,
  MyPlace,
  NameForm,
  QueueList,
  SpectatorList,
  placeAction,
  useStickToBottom,
  type SideActions,
} from "./RoomSide";

export type DrawerTab = "chat" | "players" | "you";
export const DRAWER_TABS: DrawerTab[] = ["chat", "players", "you"];

/** A request from another player to swap controllers with you. */
export interface SwapOfferView {
  from: number;
  to: number;
  name: string;
}

export interface ConsoleDrawerProps {
  open: boolean;
  onClose: () => void;
  tab: DrawerTab;
  onTab: (tab: DrawerTab) => void;
  /** Chat lines that came while the chat was out of sight. */
  unread: number;
  model: RoomModel;
  actions: SideActions;
  /** Swap controllers with a seat, or move to a free one. */
  swapFor: (port: number) => SeatSwap | undefined;
  onToggleSilence?: (port: number) => void;
  swapOffers: SwapOfferView[];
  onAnswerSwap: (from: number, to: number, accept: boolean) => void;
  fullscreen: boolean;
  onFullscreen: () => void;
  /** Opens the volume and voice settings. */
  onVoice: () => void;
  /** Opens the picture settings (style and sides). */
  onPicture?: () => void;
  onHelp: () => void;
  /** The host only: invite someone (the invitation dialog). */
  onInvite?: () => void;
  /** The host only: close the game (asks first). */
  onCloseGame?: () => void;
}

/**
 * The phone console's drawer: everything but the game, in three tabs so
 * the chat gets the whole height (a phone held sideways is short). Chat is
 * the conversation; Players, the seats, the queue and who watches; You,
 * your seat, your name and the room's buttons.
 */
export function ConsoleDrawer(props: ConsoleDrawerProps) {
  const { open, onClose, tab, onTab, unread, model, actions } = props;
  const tabRefs = useRef<Partial<Record<DrawerTab, HTMLButtonElement | null>>>({});
  const logRef = useRef<HTMLDivElement>(null);
  useStickToBottom(logRef, [tab, open]);
  // Turning the phone changes the height: keep the newest line in view.
  useEffect(() => {
    const keep = () => {
      const el = logRef.current;
      if (el) el.scrollTop = el.scrollHeight;
    };
    window.addEventListener("resize", keep);
    return () => window.removeEventListener("resize", keep);
  }, []);

  // Opening moves the focus into the drawer; closing gives it back.
  useEffect(() => {
    if (!open) return;
    const before = document.activeElement as HTMLElement | null;
    tabRefs.current[tab]?.focus({ preventScroll: true });
    return () => {
      if (before?.isConnected) before.focus({ preventScroll: true });
    };
    // only on opening: switching tabs keeps the focus where it is
  }, [open]);

  // Esc closes it, unless a sheet or a dialog over it takes the key.
  useEffect(() => {
    if (!open) return;
    const esc = (e: KeyboardEvent) => {
      if (e.key !== "Escape" && e.code !== "Escape") return;
      if (document.querySelector(".voice-pop, .dialog-backdrop")) return;
      onClose();
    };
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
  }, [open, onClose]);

  // Arrow keys, Home and End move between the tabs (a tablist's keys).
  const onTabKey = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    const i = DRAWER_TABS.indexOf(tab);
    const next =
      e.key === "ArrowRight"
        ? DRAWER_TABS[(i + 1) % DRAWER_TABS.length]
        : e.key === "ArrowLeft"
          ? DRAWER_TABS[(i + DRAWER_TABS.length - 1) % DRAWER_TABS.length]
          : e.key === "Home"
            ? DRAWER_TABS[0]
            : e.key === "End"
              ? DRAWER_TABS[DRAWER_TABS.length - 1]
              : undefined;
    if (!next) return;
    e.preventDefault();
    onTab(next);
    tabRefs.current[next]?.focus();
  };

  return (
    <div
      className={`console-drawer${open ? " is-open" : ""}`}
      role="dialog"
      aria-label={t.room.consoleMenu}
      aria-hidden={!open}
      inert={!open}
    >
      <div className="drawer-head">
        <div
          role="tablist"
          aria-label={t.room.consoleMenu}
          className="drawer-tabs"
          onKeyDown={onTabKey}
        >
          {DRAWER_TABS.map((id) => {
            const on = tab === id;
            const badge = id === "chat" && !on && unread > 0;
            return (
              <button
                key={id}
                ref={(el) => {
                  tabRefs.current[id] = el;
                }}
                type="button"
                role="tab"
                id={`drawer-tab-${id}`}
                aria-selected={on}
                aria-controls={`drawer-panel-${id}`}
                tabIndex={on ? 0 : -1}
                className={`drawer-tab${on ? " is-on" : ""}`}
                onClick={() => onTab(id)}
              >
                {t.room.drawerTabs[id]}
                {badge && (
                  <>
                    <span className="drawer-badge" aria-hidden="true">
                      {unread > 99 ? "99+" : unread}
                    </span>
                    <span className="visually-hidden">
                      {`, ${t.room.unread(unread)}`}
                    </span>
                  </>
                )}
              </button>
            );
          })}
        </div>
        <button
          type="button"
          className="icon-button drawer-close"
          aria-label={t.room.consoleClose}
          onClick={onClose}
        >
          <CloseIcon size={16} />
        </button>
      </div>

      <div
        className="drawer-panel drawer-chat"
        role="tabpanel"
        id="drawer-panel-chat"
        aria-labelledby="drawer-tab-chat"
        hidden={tab !== "chat"}
      >
        <div
          className="drawer-chat-log"
          ref={logRef}
          role="log"
          aria-label={t.room.drawerTabs.chat}
          // Scrollable with the keyboard too.
          tabIndex={0}
        >
          <ChatLog model={model} actions={actions} />
        </div>
        {!actions.chatOff && <ChatForm actions={actions} id="drawer-chatmsg" />}
      </div>

      <div
        className="drawer-panel"
        role="tabpanel"
        id="drawer-panel-players"
        aria-labelledby="drawer-tab-players"
        hidden={tab !== "players"}
      >
        <PlayersTab {...props} />
      </div>

      <div
        className="drawer-panel"
        role="tabpanel"
        id="drawer-panel-you"
        aria-labelledby="drawer-tab-you"
        hidden={tab !== "you"}
      >
        <YouTab {...props} />
      </div>
    </div>
  );
}

function PlayersTab({
  model,
  actions,
  swapFor,
  onToggleSilence,
  swapOffers,
  onAnswerSwap,
}: ConsoleDrawerProps) {
  const action = placeAction(model, actions);
  return (
    <div className="drawer-scroll">
      {swapOffers.map((o) => (
        <div
          key={`${o.from}-${o.to}`}
          className="swap-offer drawer-swap-offer"
          role="group"
          aria-label={t.room.swapOffer(localName(o.name), o.from, o.to)}
        >
          <SwapIcon />
          <span className="grow small-plus">
            {t.room.swapOffer(localName(o.name), o.from, o.to)}
          </span>
          <button
            type="button"
            className="button button-primary button-small"
            onClick={() => onAnswerSwap(o.from, o.to, true)}
          >
            {t.room.swapYes}
          </button>
          <button
            type="button"
            className="button button-secondary button-small"
            onClick={() => onAnswerSwap(o.from, o.to, false)}
          >
            {t.room.swapNo}
          </button>
        </div>
      ))}
      <ul className="drawer-seats" aria-label={t.room.playersTitle}>
        {model.seats.map((seat, i) => {
          const port = i + 1;
          const swap = swapFor(port);
          const silence =
            seat && !seat.you && onToggleSilence
              ? () => onToggleSilence(port)
              : undefined;
          return (
            <li
              key={port}
              className={`drawer-seat${seat ? "" : " is-free"}${seat?.you ? " is-you" : ""}`}
              style={portStyle(port)}
            >
              <span className="drawer-port" aria-hidden="true">
                P{port}
              </span>
              <span className="drawer-seat-name">
                <span className="visually-hidden">P{port} · </span>
                <span className="strong">
                  {seat ? seat.name : t.room.freeSeat}
                  {seat?.you ? t.room.youSuffix : ""}
                </span>
                {seat && <span className="small muted">{seat.status}</span>}
              </span>
              {swap && (
                <button
                  type="button"
                  className="button button-secondary button-small"
                  aria-label={
                    swap.waiting
                      ? t.room.swapWaiting(seat?.name ?? "")
                      : seat
                        ? t.room.swapTo(port)
                        : t.room.moveTo(port)
                  }
                  disabled={swap.waiting}
                  onClick={swap.onSwap}
                >
                  {swap.waiting
                    ? t.room.swapWaitingShort
                    : seat
                      ? t.room.swapShort
                      : t.room.moveShort}
                </button>
              )}
              {silence && seat && (
                <button
                  type="button"
                  className={`icon-button${seat.silenced ? " is-on" : ""}`}
                  aria-pressed={!!seat.silenced}
                  aria-label={t.room.silence(seat.name)}
                  onClick={silence}
                >
                  {seat.silenced ? <SoundOffIcon size={16} /> : <SoundOnIcon size={16} />}
                </button>
              )}
            </li>
          );
        })}
      </ul>
      <div className="drawer-summary">
        <span>
          {t.room.queueCount(model.queue.length, model.spectators.length)}
        </span>
        {action && (
          <button
            type="button"
            className={`button ${action.primary ? "button-primary" : "button-secondary"} button-small`}
            onClick={action.onClick}
          >
            {action.label}
          </button>
        )}
      </div>
      <h3 className="drawer-section-title">{t.room.tabs.queue}</h3>
      <QueueList model={model} tools={actions.hostTools} />
      <h3 className="drawer-section-title">{t.room.tabs.spectators}</h3>
      <SpectatorList model={model} tools={actions.hostTools} />
    </div>
  );
}

function YouTab({
  model,
  actions,
  fullscreen,
  onFullscreen,
  onVoice,
  onPicture,
  onHelp,
  onInvite,
  onCloseGame,
}: ConsoleDrawerProps) {
  return (
    <div className="drawer-scroll drawer-you">
      <MyPlace model={model} />
      {actions.onName && (
        <NameForm name={actions.name} onName={actions.onName} id="drawer-name" />
      )}
      <div className="drawer-actions">
        <button type="button" className="drawer-action" onClick={onVoice}>
          <SlidersIcon />
          <span>{t.room.voiceSettings}</span>
        </button>
        {onPicture && (
          <button type="button" className="drawer-action" onClick={onPicture}>
            <PictureIcon />
            <span>{t.picture.button}</span>
          </button>
        )}
        <button
          type="button"
          className="drawer-action"
          aria-pressed={fullscreen}
          onClick={onFullscreen}
        >
          {fullscreen ? <ExitFullscreenIcon /> : <FullscreenIcon />}
          <span>{fullscreen ? t.touch.exitFullscreen : t.room.fullscreen}</span>
        </button>
        <button type="button" className="drawer-action" onClick={onHelp}>
          <HelpIcon />
          <span>{t.help.button}</span>
        </button>
        {onInvite && (
          <button type="button" className="drawer-action is-primary" onClick={onInvite}>
            <UserPlusIcon />
            <span>{t.invite.button}</span>
          </button>
        )}
        <Link to="/rooms" className="drawer-action is-danger">
          <LogOutIcon />
          <span>{t.room.leave}</span>
        </Link>
        {onCloseGame && (
          <button type="button" className="drawer-action is-danger" onClick={onCloseGame}>
            <PowerIcon />
            <span>{t.room.closeGame}</span>
          </button>
        )}
        {actions.onSound && (
          <button
            type="button"
            className="drawer-action"
            aria-pressed={!!actions.sound}
            onClick={() => actions.onSound?.(!actions.sound)}
          >
            {actions.sound ? <BellIcon /> : <BellOffIcon />}
            <span>{t.room.chatSoundShort}</span>
          </button>
        )}
        {actions.onChatSwitch && (
          <button
            type="button"
            className="drawer-action"
            aria-pressed={!actions.chatOff}
            onClick={() => actions.onChatSwitch?.(!!actions.chatOff)}
          >
            <ChatIcon size={16} />
            <span>{t.room.roomChatShort}</span>
          </button>
        )}
      </div>
    </div>
  );
}
