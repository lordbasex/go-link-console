// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
import { getLang } from "../../i18n";
import sizes from "./shots.json";

/**
 * A real screenshot of go-link (website, device window or Player app), in
 * the page's language. The pictures live in public/shots/<lang>/<name>.webp
 * and their sizes in shots.json; most are made by `npm run shots` in e2e/
 * (the test stack's test pattern room and an invented ROM library), and the
 * app's room and chat by the iPhone app's testLiveJoin in a room of go-link's
 * own ANTÍDOTO: never someone else's game, and nothing personal.
 */
export type ShotName = keyof typeof sizes;

export function shotSrc(name: ShotName): string {
  return `/shots/${getLang()}/${name}.webp`;
}

export function shotSize(name: ShotName): { width: number; height: number } {
  const [width, height] = sizes[name] as [number, number];
  return { width, height };
}

/** A phone picture (Player app or a phone's browser), from its name. */
export function isPhoneShot(name: ShotName): boolean {
  return name.startsWith("app-") || name.startsWith("phone-");
}

export function Shot({ name, alt, className, eager }: { name: ShotName; alt: string; className?: string; eager?: boolean }) {
  return (
    <img
      className={className}
      src={shotSrc(name)}
      alt={alt}
      {...shotSize(name)}
      loading={eager ? "eager" : "lazy"}
      decoding="async"
    />
  );
}
