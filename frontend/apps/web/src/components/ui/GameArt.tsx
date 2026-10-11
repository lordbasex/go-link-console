// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

import { useState } from "react";

/**
 * A game's picture in a box that crops it to fill. A box art fills it as
 * before; a wide picture (a go-link HD game's title screen) is shown whole,
 * over a blurred copy of itself that fills the rest of the box.
 */
export function GameArt({ src, className = "" }: { src: string; className?: string }) {
  const [wide, setWide] = useState(false);
  return (
    <span className={`game-art${wide ? " is-wide" : ""}${className ? ` ${className}` : ""}`}>
      {wide && <img className="game-art-back" src={src} alt="" aria-hidden="true" />}
      <img
        className="game-art-main"
        src={src}
        alt=""
        onLoad={(e) => setWide(e.currentTarget.naturalWidth > e.currentTarget.naturalHeight * 1.2)}
      />
    </span>
  );
}
