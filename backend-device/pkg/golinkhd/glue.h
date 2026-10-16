/* Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com> */
/* C side of the go-link HD host: loads libgolinkhd and calls its API (golink_hd.h). */
#ifndef GOLINKHD_GLUE_H
#define GOLINKHD_GLUE_H

#include <stddef.h>
#include <stdint.h>
#include "golink_hd.h"

typedef struct {
	void *handle;
	const char *(*version)(void);
	int32_t (*api_version)(void);
	golinkhd_engine *(*create)(const golinkhd_config *, const char **);
	void (*destroy)(golinkhd_engine *);
	int (*load)(golinkhd_engine *, const uint8_t *, size_t, const char **);
	void (*load_demo)(golinkhd_engine *, int32_t);
	void (*get_info)(golinkhd_engine *, golinkhd_info *);
	void (*set_language)(golinkhd_engine *, const char *);
	void (*set_music)(golinkhd_engine *, int);
	void (*frame)(golinkhd_engine *, const golinkhd_pad *, int32_t, golinkhd_frame_out *);
	void (*restart)(golinkhd_engine *);
	size_t (*state_size)(golinkhd_engine *);
	int (*state_save)(golinkhd_engine *, uint8_t *, size_t);
	int (*state_load)(golinkhd_engine *, const uint8_t *, size_t, const char **);
	void (*set_resolution)(golinkhd_engine *, int32_t); /* API 2; NULL on an older engine */
	size_t (*keep_save)(golinkhd_engine *, uint8_t *, size_t);     /* API 3; NULL on an older engine */
	int (*keep_load)(golinkhd_engine *, const uint8_t *, size_t); /* API 3 */
	void (*frame_skip)(golinkhd_engine *, const golinkhd_pad *, int32_t, golinkhd_frame_out *); /* API 4; NULL on an older engine */
	void (*set_draw_budget)(golinkhd_engine *, int32_t); /* API 5; NULL on an older engine */
	int32_t (*draw_detail)(golinkhd_engine *);            /* API 5 */
	golinkhd_engine *engine;
} ghd_t;

int ghd_open(ghd_t *g, const char *path, char *err, size_t errlen);
void ghd_close(ghd_t *g);
int ghd_load(ghd_t *g, const uint8_t *data, size_t size, const char **err);
void ghd_load_demo(ghd_t *g, int32_t demo);
void ghd_info(ghd_t *g, golinkhd_info *out);
void ghd_set_language(ghd_t *g, const char *lang);
void ghd_set_music(ghd_t *g, int on);
void ghd_frame(ghd_t *g, const golinkhd_pad *pads, int32_t count, golinkhd_frame_out *out);
int ghd_frame_skip(ghd_t *g, const golinkhd_pad *pads, int32_t count, golinkhd_frame_out *out);
void ghd_restart(ghd_t *g);
int ghd_set_resolution(ghd_t *g, int32_t lines);
int ghd_set_draw_budget(ghd_t *g, int32_t microseconds);
int32_t ghd_draw_detail(ghd_t *g);
size_t ghd_state_size(ghd_t *g);
int ghd_state_save(ghd_t *g, uint8_t *out, size_t size);
int ghd_state_load(ghd_t *g, const uint8_t *in, size_t size, const char **err);
const char *ghd_version(ghd_t *g);
size_t ghd_keep_save(ghd_t *g, uint8_t *out, size_t size);
int ghd_keep_load(ghd_t *g, const uint8_t *in, size_t size);

#endif
