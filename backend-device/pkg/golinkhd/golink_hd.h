/* Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com> */
/*
 * go-link HD's API: how a host program runs the engine. go-link's device
 * (from Go), the headless runner and the tests are hosts; Willy Maker's play
 * mode will be one in WebAssembly.
 *
 * The contract, in short:
 *   - create an engine, load a game package (.glhd) or start a built-in demo;
 *   - every 1/60 s give it the players' controllers and run one frame: it
 *     returns the picture (XRGB8888) and the sound of that frame (48 kHz
 *     stereo, 800 samples);
 *   - save and load its state at any frame: the same state and the same
 *     controllers give the same pictures and sound on every computer.
 *
 * Compatibility: GOLINKHD_API_VERSION grows when the API grows. A host asks
 * for the version it was written for (golinkhd_config.api_version) and an
 * engine serves every older one; functions are only added, never changed.
 * Save states and packages carry versions of their own (see the README).
 *
 * The idea of a small, frontend-agnostic contract (load, run a frame, give a
 * picture and sound, serialize the state) comes from libretro's API, which
 * this engine followed at first; it now has its own, so it can grow with
 * what go-link needs (events for the room, views per player, several engines
 * in one program).
 */
#ifndef GOLINK_HD_H
#define GOLINK_HD_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define GOLINKHD_API_VERSION 3
#define GOLINKHD_KEEP_MAX 256 /* API 3: the most a game keeps between sessions (golinkhd_keep_save) */

#if defined(_WIN32) && defined(GOLINKHD_BUILD_SHARED)
#define GOLINKHD_API __declspec(dllexport)
#elif defined(__GNUC__) || defined(__clang__)
#define GOLINKHD_API __attribute__((visibility("default")))
#else
#define GOLINKHD_API
#endif

/* The most players a game can take. */
#define GOLINKHD_MAX_PLAYERS 8

/* A controller's buttons. */
enum
{
   GOLINKHD_UP = 1 << 0,
   GOLINKHD_DOWN = 1 << 1,
   GOLINKHD_LEFT = 1 << 2,
   GOLINKHD_RIGHT = 1 << 3,
   GOLINKHD_JUMP = 1 << 4,  /* the game's main action: B or A on a pad */
   GOLINKHD_RUN = 1 << 5,   /* the second action: Y or X */
   GOLINKHD_START = 1 << 6,
   GOLINKHD_SELECT = 1 << 7,
   GOLINKHD_L = 1 << 8,
   GOLINKHD_R = 1 << 9,
   GOLINKHD_L2 = 1 << 10,
   GOLINKHD_R2 = 1 << 11,
   GOLINKHD_L3 = 1 << 12,
   GOLINKHD_R3 = 1 << 13,
   /* each face button on its own, for games that tell them apart (bottom, right, left, top) */
   GOLINKHD_A = 1 << 14,
   GOLINKHD_B = 1 << 15,
   GOLINKHD_X = 1 << 16,
   GOLINKHD_Y = 1 << 17
};

/* One player's controller for one frame. Sticks are -32768..32767, right and down positive. */
typedef struct
{
   uint32_t buttons;
   int32_t lx, ly, rx, ry;
} golinkhd_pad;

enum { GOLINKHD_LOG_DEBUG = 0, GOLINKHD_LOG_INFO, GOLINKHD_LOG_WARN, GOLINKHD_LOG_ERROR };

typedef struct
{
   int32_t api_version; /* GOLINKHD_API_VERSION, as the host knows it */
   /* where the engine's messages go (may be NULL) */
   void (*log)(void *user, int32_t level, const char *message);
   void *user;
} golinkhd_config;

/* What the loaded game is. */
typedef struct
{
   const char *title;
   int32_t width, height; /* the logical screen: 640 x 360, 480 x 360 or 360 x 640 */
   int32_t fps;           /* 60 */
   int32_t sample_rate;   /* 48000 */
   int32_t players;       /* how many it takes, 1 to 8 */
   uint8_t sha256[32];    /* the package's; zeros for a built-in demo */
} golinkhd_info;

/* One frame's output, valid until the next call on the engine. */
typedef struct
{
   const uint32_t *pixels; /* 0x00RRGGBB, rows of `pitch` pixels */
   int32_t width, height, pitch;
   const int16_t *audio;   /* interleaved stereo */
   int32_t audio_frames;   /* stereo samples: 800 at 60 fps */
} golinkhd_frame_out;

typedef struct golinkhd_engine golinkhd_engine;

/* The engine's own version, e.g. "0.2.0". */
GOLINKHD_API const char *golinkhd_version(void);
/* The newest API version this engine serves. */
GOLINKHD_API int32_t golinkhd_api_version(void);

/*
 * A new engine, playing the built-in platformer demo; NULL and *error when
 * the host asks for an API this engine does not know, or (in this version)
 * when an engine already exists in this program.
 */
GOLINKHD_API golinkhd_engine *golinkhd_create(const golinkhd_config *config, const char **error);
GOLINKHD_API void golinkhd_destroy(golinkhd_engine *e);

/* A game package (.glhd) in memory; 0 and a readable *error when it cannot be played (the engine keeps its demo). */
GOLINKHD_API int golinkhd_load(golinkhd_engine *e, const uint8_t *package, size_t size, const char **error);
/* Back to a built-in demo: 0 the platformer, 1 the showcase of every effect, 2 the 3D room, 3 the 3D game TELETRABAJO: CÁMARA APAGADA (2 and 3 are the platformer on an engine built without 3D). */
GOLINKHD_API void golinkhd_load_demo(golinkhd_engine *e, int32_t demo);
GOLINKHD_API void golinkhd_get_info(golinkhd_engine *e, golinkhd_info *out);

/* Settings: the language of texts and dialogs ("en", "es", "pt") and the music (on or off). */
GOLINKHD_API void golinkhd_set_language(golinkhd_engine *e, const char *language);
GOLINKHD_API void golinkhd_set_music(golinkhd_engine *e, int on);
/*
 * API 2: the picture's size for the next golinkhd_load, in lines (360, 720
 * or 1080; 0, the default, is the package's own "resolution"). A package
 * draws it from pictures painted that big or bigger, made smaller as it
 * loads, so one package serves every size; it is never drawn bigger than
 * its pictures (golinkhd_frame_out says what it is).
 */
GOLINKHD_API void golinkhd_set_resolution(golinkhd_engine *e, int32_t lines);

/* One frame: the controllers of ports 1..count (the rest are idle), then the picture and sound. */
GOLINKHD_API void golinkhd_frame(golinkhd_engine *e, const golinkhd_pad *pads, int32_t count, golinkhd_frame_out *out);
/* The game from its start (the loaded package or demo). */
GOLINKHD_API void golinkhd_restart(golinkhd_engine *e);

/* Save states: their size, a save into out (size bytes), a load (0 and *error when it is not for this game or version). */
GOLINKHD_API size_t golinkhd_state_size(golinkhd_engine *e);
GOLINKHD_API int golinkhd_state_save(golinkhd_engine *e, uint8_t *out, size_t size);
GOLINKHD_API int golinkhd_state_load(golinkhd_engine *e, const uint8_t *in, size_t size, const char **error);

/*
 * API 3: what the game keeps between sessions (its record, the player's
 * choices), a few bytes the host stores for this game and gives back. A save
 * writes them into out (at most GOLINKHD_KEEP_MAX) and returns how many, 0
 * when the game keeps nothing; they change as it is played, so the host asks
 * again now and then (when a room stops, every few minutes). A load gives
 * them back after golinkhd_load (and survives golinkhd_restart); 0 when they
 * are not this game's (the game starts without them).
 */
GOLINKHD_API size_t golinkhd_keep_save(golinkhd_engine *e, uint8_t *out, size_t size);
GOLINKHD_API int golinkhd_keep_load(golinkhd_engine *e, const uint8_t *in, size_t size);

#ifdef __cplusplus
}
#endif

#endif
