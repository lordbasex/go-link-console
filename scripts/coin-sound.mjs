// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>
//
// Writes the startup coin sound of the go-link Console Player apps as a WAV file
// (44.1 kHz, mono, 16-bit). It is an original sound made here, MIT licensed
// like the rest of the project: three quick square-wave notes, the same
// synthesis the website plays with Web Audio (frontend/apps/web/src/coin.ts).
//
//   node scripts/coin-sound.mjs
//
// writes mobile/ios/GoLinkPlayer/Resources/coin.wav and
// mobile/android/app/src/main/res/raw/coin.wav (both committed).

import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const RATE = 44100;
// Note i starts at i * STEP s, plays 880 Hz * 1.26^i for 0.12 + i * 0.08 s,
// with a gain of 0.12 that decays exponentially to 0.0001.
const STEP = 0.07;
const NOTES = [0, 1, 2].map((i) => ({
  start: i * STEP,
  freq: 880 * Math.pow(1.26, i),
  dur: 0.12 + i * 0.08,
}));
const GAIN = 0.12;
const FLOOR = 0.0001;
// Web Audio stops each oscillator 20 ms after its ramp ends.
const TAIL = 0.02;

/** A band-limited square wave (odd harmonics below 20 kHz), like Web Audio's. */
function square(freq, t) {
  let sum = 0;
  for (let k = 1; k * freq < 20000; k += 2) sum += Math.sin(2 * Math.PI * k * freq * t) / k;
  return (4 / Math.PI) * sum;
}

function render() {
  const length = Math.max(...NOTES.map((n) => n.start + n.dur + TAIL));
  const samples = new Float64Array(Math.ceil(length * RATE));
  for (const n of NOTES) {
    const first = Math.round(n.start * RATE);
    const count = Math.round((n.dur + TAIL) * RATE);
    for (let j = 0; j < count && first + j < samples.length; j++) {
      const t = j / RATE;
      // setValueAtTime(GAIN) then exponentialRampToValueAtTime(FLOOR, dur).
      const gain = t < n.dur ? GAIN * Math.pow(FLOOR / GAIN, t / n.dur) : FLOOR;
      samples[first + j] += square(n.freq, t) * gain;
    }
  }
  // A 5 ms fade at the very end so the file never ends on a click.
  const fade = Math.round(0.005 * RATE);
  for (let j = 0; j < fade; j++) samples[samples.length - 1 - j] *= j / fade;
  return samples;
}

function wav(samples) {
  const data = Buffer.alloc(samples.length * 2);
  samples.forEach((s, i) => data.writeInt16LE(Math.round(Math.max(-1, Math.min(1, s)) * 32767), i * 2));
  const head = Buffer.alloc(44);
  head.write("RIFF", 0, "ascii");
  head.writeUInt32LE(36 + data.length, 4);
  head.write("WAVE", 8, "ascii");
  head.write("fmt ", 12, "ascii");
  head.writeUInt32LE(16, 16); // PCM header size
  head.writeUInt16LE(1, 20); // PCM
  head.writeUInt16LE(1, 22); // mono
  head.writeUInt32LE(RATE, 24);
  head.writeUInt32LE(RATE * 2, 28); // bytes per second
  head.writeUInt16LE(2, 32); // bytes per frame
  head.writeUInt16LE(16, 34); // bits per sample
  head.write("data", 36, "ascii");
  head.writeUInt32LE(data.length, 40);
  return Buffer.concat([head, data]);
}

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const file = wav(render());
for (const out of [
  "mobile/ios/GoLinkPlayer/Resources/coin.wav",
  "mobile/android/app/src/main/res/raw/coin.wav",
]) {
  mkdirSync(dirname(join(root, out)), { recursive: true });
  writeFileSync(join(root, out), file);
  console.log(`${out}: ${file.length} bytes`);
}
