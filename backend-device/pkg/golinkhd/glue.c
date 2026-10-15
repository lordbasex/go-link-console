/* Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com> */
#include <stdio.h>
#include <string.h>
#include "glue.h"

#ifdef _WIN32
#include <windows.h>
static void *lib_open(const char *p) { return (void *)LoadLibraryA(p); }
static void *lib_sym(void *h, const char *n) { return (void *)GetProcAddress((HMODULE)h, n); }
static void lib_close(void *h) { FreeLibrary((HMODULE)h); }
static const char *lib_error(void) { return "LoadLibrary failed"; }
#else
#include <dlfcn.h>
static void *lib_open(const char *p) { return dlopen(p, RTLD_NOW | RTLD_LOCAL); }
static void *lib_sym(void *h, const char *n) { return dlsym(h, n); }
static void lib_close(void *h) { dlclose(h); }
static const char *lib_error(void) { const char *e = dlerror(); return e ? e : "unknown error"; }
#endif

#define LOAD(field, name)                                            \
	do {                                                             \
		*(void **)(&g->field) = lib_sym(g->handle, name);            \
		if (!g->field) {                                             \
			snprintf(err, errlen, "the library has no %s", name);    \
			lib_close(g->handle);                                    \
			g->handle = NULL;                                        \
			return -2;                                               \
		}                                                            \
	} while (0)

int ghd_open(ghd_t *g, const char *path, char *err, size_t errlen) {
	golinkhd_config cfg;
	const char *why = NULL;
	memset(g, 0, sizeof(*g));
	g->handle = lib_open(path);
	if (!g->handle) {
		snprintf(err, errlen, "%s", lib_error());
		return -1;
	}
	LOAD(version, "golinkhd_version");
	LOAD(api_version, "golinkhd_api_version");
	LOAD(create, "golinkhd_create");
	LOAD(destroy, "golinkhd_destroy");
	LOAD(load, "golinkhd_load");
	LOAD(load_demo, "golinkhd_load_demo");
	LOAD(get_info, "golinkhd_get_info");
	LOAD(set_language, "golinkhd_set_language");
	LOAD(set_music, "golinkhd_set_music");
	LOAD(frame, "golinkhd_frame");
	LOAD(restart, "golinkhd_restart");
	LOAD(state_size, "golinkhd_state_size");
	LOAD(state_save, "golinkhd_state_save");
	LOAD(state_load, "golinkhd_state_load");
	*(void **)(&g->set_resolution) = lib_sym(g->handle, "golinkhd_set_resolution"); /* optional */
	*(void **)(&g->keep_save) = lib_sym(g->handle, "golinkhd_keep_save");           /* optional */
	*(void **)(&g->keep_load) = lib_sym(g->handle, "golinkhd_keep_load");
	*(void **)(&g->frame_skip) = lib_sym(g->handle, "golinkhd_frame_skip");         /* optional */
	memset(&cfg, 0, sizeof(cfg));
	cfg.api_version = 2; /* the oldest API this host needs: newer functions are looked up and optional */
	g->engine = g->create(&cfg, &why);
	if (!g->engine) {
		snprintf(err, errlen, "%s", why ? why : "the engine did not start");
		lib_close(g->handle);
		g->handle = NULL;
		return -3;
	}
	return 0;
}

void ghd_close(ghd_t *g) {
	if (g->engine)
		g->destroy(g->engine);
	if (g->handle)
		lib_close(g->handle);
	memset(g, 0, sizeof(*g));
}

int ghd_load(ghd_t *g, const uint8_t *data, size_t size, const char **err) { return g->load(g->engine, data, size, err); }
void ghd_load_demo(ghd_t *g, int32_t demo) { g->load_demo(g->engine, demo); }
void ghd_info(ghd_t *g, golinkhd_info *out) { g->get_info(g->engine, out); }
void ghd_set_language(ghd_t *g, const char *lang) { g->set_language(g->engine, lang); }
void ghd_set_music(ghd_t *g, int on) { g->set_music(g->engine, on); }
void ghd_frame(ghd_t *g, const golinkhd_pad *pads, int32_t count, golinkhd_frame_out *out) { g->frame(g->engine, pads, count, out); }
int ghd_frame_skip(ghd_t *g, const golinkhd_pad *pads, int32_t count, golinkhd_frame_out *out) {
	if (!g->frame_skip)
		return 0;
	g->frame_skip(g->engine, pads, count, out);
	return 1;
}
void ghd_restart(ghd_t *g) { g->restart(g->engine); }
int ghd_set_resolution(ghd_t *g, int32_t lines) {
	if (!g->set_resolution)
		return 0;
	g->set_resolution(g->engine, lines);
	return 1;
}
size_t ghd_state_size(ghd_t *g) { return g->state_size(g->engine); }
int ghd_state_save(ghd_t *g, uint8_t *out, size_t size) { return g->state_save(g->engine, out, size); }
int ghd_state_load(ghd_t *g, const uint8_t *in, size_t size, const char **err) { return g->state_load(g->engine, in, size, err); }
const char *ghd_version(ghd_t *g) { return g->version(); }
size_t ghd_keep_save(ghd_t *g, uint8_t *out, size_t size) {
	if (!g->keep_save || !g->keep_load)
		return 0;
	return g->keep_save(g->engine, out, size);
}
int ghd_keep_load(ghd_t *g, const uint8_t *in, size_t size) {
	if (!g->keep_save || !g->keep_load)
		return 0;
	return g->keep_load(g->engine, in, size);
}
