// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && cgo

package encoder

/*
#cgo LDFLAGS: -lmfplat -lmfuuid -lole32 -loleaut32 -luuid
#define COBJMACROS
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <windows.h>
#include <mfapi.h>
#include <mfidl.h>
#include <mftransform.h>
#include <mferror.h>
#include <codecapi.h>
#include <strmif.h>

extern void goMFFrame(uintptr_t handle, void *data, int len);

// Every GUID used, defined here (the import libraries do not have all of them).
static const GUID g_video = { 0x73646976, 0x0000, 0x0010, { 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71 } };   // MFMediaType_Video
static const GUID g_nv12 = { 0x3231564e, 0x0000, 0x0010, { 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71 } };    // MFVideoFormat_NV12
static const GUID g_h264 = { 0x34363248, 0x0000, 0x0010, { 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71 } };    // MFVideoFormat_H264
static const GUID g_enc = { 0xf79eac7d, 0xe545, 0x4387, { 0xbd, 0xee, 0xd6, 0x47, 0xd7, 0xbd, 0xe4, 0x2a } };     // MFT_CATEGORY_VIDEO_ENCODER
static const GUID g_major = { 0x48eba18e, 0xf8c9, 0x4687, { 0xbf, 0x11, 0x0a, 0x74, 0xc9, 0xf9, 0x6a, 0x8f } };   // MF_MT_MAJOR_TYPE
static const GUID g_subtype = { 0xf7e34c9a, 0x42e8, 0x4714, { 0xb7, 0x4b, 0xcb, 0x29, 0xd7, 0x2c, 0x35, 0xe5 } }; // MF_MT_SUBTYPE
static const GUID g_size = { 0x1652c33d, 0xd6b2, 0x4012, { 0xb8, 0x34, 0x72, 0x03, 0x08, 0x49, 0xa3, 0x7d } };    // MF_MT_FRAME_SIZE
static const GUID g_rate = { 0xc459a2e8, 0x3d2c, 0x4e44, { 0xb1, 0x32, 0xfe, 0xe5, 0x15, 0x6c, 0x7b, 0xb0 } };    // MF_MT_FRAME_RATE
static const GUID g_bitrate = { 0x20332624, 0xfb0d, 0x4d9e, { 0xbd, 0x0d, 0xcb, 0xf6, 0x78, 0x6c, 0x10, 0x2e } }; // MF_MT_AVG_BITRATE
static const GUID g_interlace = { 0xe2724bb8, 0xe676, 0x4806, { 0xb4, 0xb2, 0xa8, 0xd6, 0xef, 0xb4, 0x4c, 0xcd } }; // MF_MT_INTERLACE_MODE
static const GUID g_profile = { 0xad76a80b, 0x2d5c, 0x4e0b, { 0xb3, 0x75, 0x64, 0xe5, 0x20, 0x13, 0x70, 0x36 } }; // MF_MT_MPEG2_PROFILE
static const GUID g_par = { 0xc6376a1e, 0x8d0a, 0x4027, { 0xbe, 0x45, 0x6d, 0x9a, 0x0a, 0xd3, 0x9b, 0xb6 } };     // MF_MT_PIXEL_ASPECT_RATIO
static const GUID g_unlock = { 0xe5666d6b, 0x3422, 0x4eb6, { 0xa4, 0x21, 0xda, 0x7d, 0xb1, 0xf8, 0xe2, 0x07 } };  // MF_TRANSFORM_ASYNC_UNLOCK
static const GUID g_async = { 0xf81a699a, 0x649a, 0x497d, { 0x8c, 0x73, 0x29, 0xf8, 0xfe, 0xd6, 0xad, 0x7a } };   // MF_TRANSFORM_ASYNC
static const GUID g_name = { 0x314ffbae, 0x5b41, 0x4c95, { 0x9c, 0x19, 0x4e, 0x7d, 0x58, 0x6f, 0xac, 0xe3 } };    // MFT_FRIENDLY_NAME_Attribute
static const GUID g_clean = { 0x9cdf01d8, 0xa0f0, 0x43ba, { 0xb0, 0x77, 0xea, 0xa0, 0x6c, 0xbd, 0x72, 0x8a } };   // MFSampleExtension_CleanPoint
static const GUID g_codecapi = { 0x901db4c7, 0x31ce, 0x41a2, { 0x85, 0xdc, 0x8f, 0xa0, 0xbf, 0x41, 0xb8, 0xda } }; // IID_ICodecAPI
static const GUID g_rc = { STATIC_CODECAPI_AVEncCommonRateControlMode };
static const GUID g_mean = { STATIC_CODECAPI_AVEncCommonMeanBitRate };
static const GUID g_gop = { STATIC_CODECAPI_AVEncMPVGOPSize };
static const GUID g_bframes = { STATIC_CODECAPI_AVEncMPVDefaultBPictureCount };
static const GUID g_lowlat = { STATIC_CODECAPI_AVLowLatencyMode };
static const GUID g_forcekey = { STATIC_CODECAPI_AVEncVideoForceKeyFrame };

typedef struct {
	IMFActivate *act;
	IMFTransform *mft;
	IMFMediaEventGenerator *events; // NULL: a synchronous encoder
	ICodecAPI *api;
	DWORD in_id, out_id;
	int w, h, fps, need, provides, out_size, hardware;
	int64_t n;
	uintptr_t handle;
	uint8_t *seq; // the SPS and PPS (Annex B), put before a keyframe that has none
	int seq_n;
	char name[128];
} mf_enc;

static void set_u32(mf_enc *e, const GUID *key, ULONG v) {
	VARIANT var;
	if (!e->api)
		return;
	VariantInit(&var);
	var.vt = VT_UI4;
	var.ulVal = v;
	ICodecAPI_SetValue(e->api, key, &var); // a setting an encoder does not have is left as it is
}

// The output type: H.264 baseline at the size, rate and bitrate; then the input type, NV12.
static HRESULT set_types(mf_enc *e, int kbps) {
	IMFMediaType *t = NULL;
	HRESULT hr = MFCreateMediaType(&t);
	if (FAILED(hr))
		return hr;
	IMFMediaType_SetGUID(t, &g_major, &g_video);
	IMFMediaType_SetGUID(t, &g_subtype, &g_h264);
	IMFMediaType_SetUINT32(t, &g_bitrate, (UINT32)kbps * 1000);
	IMFMediaType_SetUINT64(t, &g_size, ((UINT64)e->w << 32) | (UINT32)e->h);
	IMFMediaType_SetUINT64(t, &g_rate, ((UINT64)e->fps << 32) | 1);
	IMFMediaType_SetUINT64(t, &g_par, ((UINT64)1 << 32) | 1);
	IMFMediaType_SetUINT32(t, &g_interlace, 2); // MFVideoInterlace_Progressive
	IMFMediaType_SetUINT32(t, &g_profile, eAVEncH264VProfile_Base);
	hr = IMFTransform_SetOutputType(e->mft, e->out_id, t, 0);
	IMFMediaType_Release(t);
	if (FAILED(hr))
		return hr;
	hr = MFCreateMediaType(&t);
	if (FAILED(hr))
		return hr;
	IMFMediaType_SetGUID(t, &g_major, &g_video);
	IMFMediaType_SetGUID(t, &g_subtype, &g_nv12);
	IMFMediaType_SetUINT64(t, &g_size, ((UINT64)e->w << 32) | (UINT32)e->h);
	IMFMediaType_SetUINT64(t, &g_rate, ((UINT64)e->fps << 32) | 1);
	IMFMediaType_SetUINT64(t, &g_par, ((UINT64)1 << 32) | 1);
	IMFMediaType_SetUINT32(t, &g_interlace, 2);
	hr = IMFTransform_SetInputType(e->mft, e->in_id, t, 0);
	IMFMediaType_Release(t);
	return hr;
}

static void put_start(uint8_t **p) { (*p)[0] = 0; (*p)[1] = 0; (*p)[2] = 0; (*p)[3] = 1; *p += 4; }

// One encoded frame (Annex B from the encoder) as one access unit: an access
// unit delimiter, the SPS and PPS before a keyframe that lacks them, the frame.
static void emit(mf_enc *e, const uint8_t *d, DWORD n, int key) {
	int has_sps = 0;
	DWORD i;
	for (i = 0; i + 4 < n; i++)
		if (d[i] == 0 && d[i + 1] == 0 && d[i + 2] == 1) {
			uint8_t t = d[i + 3] & 31;
			if (t == 7)
				has_sps = 1;
			if (t == 5)
				key = 1;
		}
	uint8_t *out = (uint8_t *)malloc(6 + e->seq_n + n);
	if (!out)
		return;
	uint8_t *p = out;
	put_start(&p);
	*p++ = 0x09; // access unit delimiter
	*p++ = 0xf0;
	if (key && !has_sps && e->seq_n) {
		memcpy(p, e->seq, e->seq_n);
		p += e->seq_n;
	}
	memcpy(p, d, n);
	p += n;
	goMFFrame(e->handle, out, (int)(p - out));
	free(out);
}

// The sequence header (SPS and PPS) the encoder put on its output type, kept once.
static void keep_seq(mf_enc *e) {
	IMFMediaType *t = NULL;
	UINT32 n = 0;
	static const GUID seqh = { 0x3c036de7, 0x3ad0, 0x4c9e, { 0x92, 0x16, 0xee, 0x6d, 0x6a, 0xc2, 0x1c, 0xb3 } }; // MF_MT_MPEG_SEQUENCE_HEADER
	if (e->seq_n || FAILED(IMFTransform_GetOutputCurrentType(e->mft, e->out_id, &t)))
		return;
	if (SUCCEEDED(IMFMediaType_GetBlobSize(t, &seqh, &n)) && n > 0 && n < 4096) {
		e->seq = (uint8_t *)malloc(n);
		if (e->seq && SUCCEEDED(IMFMediaType_GetBlob(t, &seqh, e->seq, n, NULL)))
			e->seq_n = (int)n;
	}
	IMFMediaType_Release(t);
}

// Takes one frame out of the encoder; S_OK with one emitted, MF_E_TRANSFORM_NEED_MORE_INPUT when it has none.
static HRESULT pull(mf_enc *e) {
	MFT_OUTPUT_DATA_BUFFER ob;
	DWORD status = 0;
	IMFSample *s = NULL;
	IMFMediaBuffer *b = NULL;
	HRESULT hr;
	memset(&ob, 0, sizeof ob);
	ob.dwStreamID = e->out_id;
	if (!e->provides) {
		if (FAILED(MFCreateSample(&s)) || FAILED(MFCreateMemoryBuffer(e->out_size, &b)))
			return E_OUTOFMEMORY;
		IMFSample_AddBuffer(s, b);
		IMFMediaBuffer_Release(b);
		ob.pSample = s;
	}
	hr = IMFTransform_ProcessOutput(e->mft, 0, 1, &ob, &status);
	if (hr == MF_E_TRANSFORM_STREAM_CHANGE) {
		// a hardware encoder asks for its output type again: take the first one it offers
		IMFMediaType *t = NULL;
		if (ob.pEvents)
			IMFCollection_Release(ob.pEvents);
		if (s)
			IMFSample_Release(s);
		if (SUCCEEDED(IMFTransform_GetOutputAvailableType(e->mft, e->out_id, 0, &t))) {
			hr = IMFTransform_SetOutputType(e->mft, e->out_id, t, 0);
			IMFMediaType_Release(t);
		}
		return FAILED(hr) ? hr : S_FALSE;
	}
	if (ob.pEvents)
		IMFCollection_Release(ob.pEvents);
	if (SUCCEEDED(hr) && ob.pSample) {
		IMFMediaBuffer *c = NULL;
		BYTE *data;
		DWORD len;
		UINT32 clean = 0;
		IMFSample_GetUINT32(ob.pSample, &g_clean, &clean);
		keep_seq(e);
		if (SUCCEEDED(IMFSample_ConvertToContiguousBuffer(ob.pSample, &c))) {
			if (SUCCEEDED(IMFMediaBuffer_Lock(c, &data, NULL, &len))) {
				if (len)
					emit(e, data, len, clean != 0);
				IMFMediaBuffer_Unlock(c);
			}
			IMFMediaBuffer_Release(c);
		}
	}
	if (ob.pSample)
		IMFSample_Release(ob.pSample);
	return hr;
}

// Waits for (block) or checks the encoder's events: each output is pulled, each input it asks for counted.
static HRESULT events(mf_enc *e, int block) {
	for (;;) {
		IMFMediaEvent *ev = NULL;
		MediaEventType type;
		HRESULT hr = IMFMediaEventGenerator_GetEvent(e->events, block ? 0 : MF_EVENT_FLAG_NO_WAIT, &ev);
		if (hr == MF_E_NO_EVENTS_AVAILABLE)
			return S_OK;
		if (FAILED(hr))
			return hr;
		IMFMediaEvent_GetType(ev, &type);
		IMFMediaEvent_Release(ev);
		if (type == METransformNeedInput)
			e->need++;
		else if (type == METransformHaveOutput) {
			hr = pull(e);
			if (FAILED(hr) && hr != MF_E_TRANSFORM_NEED_MORE_INPUT)
				return hr;
		}
		if (block)
			return S_OK; // one event; the caller loops until it can go on
	}
}

static HRESULT start(mf_enc *e, int kbps, int gop) {
	IMFAttributes *a = NULL;
	MFT_OUTPUT_STREAM_INFO info;
	UINT32 async = 0;
	HRESULT hr = IMFActivate_ActivateObject(e->act, &IID_IMFTransform, (void **)&e->mft);
	if (FAILED(hr))
		return hr;
	if (SUCCEEDED(IMFTransform_GetAttributes(e->mft, &a))) {
		IMFAttributes_GetUINT32(a, &g_async, &async);
		if (async)
			IMFAttributes_SetUINT32(a, &g_unlock, TRUE); // a hardware encoder is asynchronous: unlocked to be used
		IMFAttributes_Release(a);
	}
	if (async && FAILED(IMFTransform_QueryInterface(e->mft, &IID_IMFMediaEventGenerator, (void **)&e->events)))
		return E_NOINTERFACE;
	if (IMFTransform_GetStreamIDs(e->mft, 1, &e->in_id, 1, &e->out_id) == E_NOTIMPL)
		e->in_id = e->out_id = 0;
	IMFTransform_QueryInterface(e->mft, &g_codecapi, (void **)&e->api);
	set_u32(e, &g_rc, eAVEncCommonRateControlMode_CBR);
	set_u32(e, &g_mean, (ULONG)kbps * 1000);
	set_u32(e, &g_gop, (ULONG)gop);
	set_u32(e, &g_bframes, 0);
	if (e->api) {
		VARIANT v;
		VariantInit(&v);
		v.vt = VT_BOOL;
		v.boolVal = VARIANT_TRUE;
		ICodecAPI_SetValue(e->api, &g_lowlat, &v);
	}
	hr = set_types(e, kbps);
	if (FAILED(hr))
		return hr;
	if (FAILED(hr = IMFTransform_GetOutputStreamInfo(e->mft, e->out_id, &info)))
		return hr;
	e->provides = (info.dwFlags & (MFT_OUTPUT_STREAM_PROVIDES_SAMPLES | MFT_OUTPUT_STREAM_CAN_PROVIDE_SAMPLES)) != 0;
	e->out_size = info.cbSize ? (int)info.cbSize : e->w * e->h * 3 / 2;
	IMFTransform_ProcessMessage(e->mft, MFT_MESSAGE_COMMAND_FLUSH, 0);
	IMFTransform_ProcessMessage(e->mft, MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0);
	IMFTransform_ProcessMessage(e->mft, MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0);
	return S_OK;
}

static void stop(mf_enc *e) {
	if (e->mft) {
		IMFTransform_ProcessMessage(e->mft, MFT_MESSAGE_NOTIFY_END_OF_STREAM, 0);
		IMFTransform_ProcessMessage(e->mft, MFT_MESSAGE_COMMAND_FLUSH, 0);
	}
	if (e->api)
		ICodecAPI_Release(e->api);
	if (e->events)
		IMFMediaEventGenerator_Release(e->events);
	if (e->mft)
		IMFTransform_Release(e->mft);
	if (e->act) {
		IMFActivate_ShutdownObject(e->act);
		IMFActivate_Release(e->act);
	}
	e->api = NULL;
	e->events = NULL;
	e->mft = NULL;
	e->act = NULL;
}

// The H.264 encoders Windows lists for NV12: hardware first (best first), else software.
static UINT32 find(int hardware, IMFActivate ***list) {
	MFT_REGISTER_TYPE_INFO in = { g_video, g_nv12 }, out = { g_video, g_h264 };
	UINT32 n = 0;
	UINT32 flags = (hardware ? MFT_ENUM_FLAG_HARDWARE : MFT_ENUM_FLAG_SYNCMFT) | MFT_ENUM_FLAG_SORTANDFILTER;
	if (FAILED(MFTEnumEx(g_enc, flags, &in, &out, list, &n)))
		return 0;
	return n;
}

static int mf_init(void) {
	HRESULT hr = CoInitializeEx(NULL, COINIT_MULTITHREADED);
	if (FAILED(hr) && hr != RPC_E_CHANGED_MODE)
		return (int)hr;
	return (int)MFStartup(MF_VERSION, MFSTARTUP_NOSOCKET);
}

static void mf_done(void) {
	MFShutdown();
	CoUninitialize();
}

// The first hardware encoder's name, empty when there is none.
static void mf_hardware_name(char *out, int cap) {
	IMFActivate **list = NULL;
	UINT32 n = find(1, &list), i;
	WCHAR *w = NULL;
	UINT32 len;
	out[0] = 0;
	if (n && SUCCEEDED(IMFActivate_GetAllocatedString(list[0], &g_name, &w, &len))) {
		WideCharToMultiByte(CP_UTF8, 0, w, -1, out, cap, NULL, NULL);
		CoTaskMemFree(w);
	}
	for (i = 0; i < n; i++)
		IMFActivate_Release(list[i]);
	CoTaskMemFree(list);
}

// An encoder: the first hardware one that takes the size, else (hardware 0) Windows' software one.
static mf_enc *mf_open(int w, int h, int fps, int kbps, int gop, int hardware, uintptr_t handle, int *err) {
	mf_enc *e = (mf_enc *)calloc(1, sizeof *e);
	IMFActivate **list = NULL;
	UINT32 n, i;
	HRESULT hr = E_FAIL;
	if (!e) {
		*err = -1;
		return NULL;
	}
	e->w = w;
	e->h = h;
	e->fps = fps;
	e->handle = handle;
	n = find(hardware, &list);
	for (i = 0; i < n; i++) {
		if (!e->mft) {
			e->act = list[i];
			IMFActivate_AddRef(e->act);
			hr = start(e, kbps, gop);
			if (FAILED(hr))
				stop(e); // this one refuses the size or the type: the next
			else {
				WCHAR *nm = NULL;
				UINT32 len;
				e->hardware = hardware;
				if (SUCCEEDED(IMFActivate_GetAllocatedString(list[i], &g_name, &nm, &len))) {
					WideCharToMultiByte(CP_UTF8, 0, nm, -1, e->name, sizeof e->name, NULL, NULL);
					CoTaskMemFree(nm);
				}
			}
		}
		IMFActivate_Release(list[i]);
	}
	CoTaskMemFree(list);
	if (!e->mft) {
		*err = n ? (int)hr : -2;
		free(e);
		return NULL;
	}
	*err = 0;
	return e;
}

// One I420 frame in, copied into an NV12 sample.
static int mf_encode(mf_enc *e, const uint8_t *i420, int key) {
	IMFSample *s = NULL;
	IMFMediaBuffer *b = NULL;
	BYTE *d;
	int w = e->w, h = e->h, x, y;
	DWORD size = (DWORD)(w * h * 3 / 2);
	HRESULT hr;
	if (FAILED(hr = MFCreateMemoryBuffer(size, &b)))
		return (int)hr;
	if (FAILED(hr = IMFMediaBuffer_Lock(b, &d, NULL, NULL))) {
		IMFMediaBuffer_Release(b);
		return (int)hr;
	}
	memcpy(d, i420, (size_t)w * h);
	{
		const uint8_t *u = i420 + (size_t)w * h, *v = u + (size_t)(w / 2) * (h / 2);
		uint8_t *uv = d + (size_t)w * h;
		for (y = 0; y < h / 2; y++)
			for (x = 0; x < w / 2; x++) {
				uv[y * w + 2 * x] = u[y * (w / 2) + x];
				uv[y * w + 2 * x + 1] = v[y * (w / 2) + x];
			}
	}
	IMFMediaBuffer_Unlock(b);
	IMFMediaBuffer_SetCurrentLength(b, size);
	if (FAILED(hr = MFCreateSample(&s))) {
		IMFMediaBuffer_Release(b);
		return (int)hr;
	}
	IMFSample_AddBuffer(s, b);
	IMFMediaBuffer_Release(b);
	IMFSample_SetSampleTime(s, e->n * 10000000LL / e->fps);
	IMFSample_SetSampleDuration(s, 10000000LL / e->fps);
	e->n++;
	if (key)
		set_u32(e, &g_forcekey, 1);
	if (e->events) {
		// asynchronous: input only when the encoder asked for it, outputs as they come
		while (e->need == 0)
			if (FAILED(hr = events(e, 1)))
				break;
		if (SUCCEEDED(hr)) {
			hr = IMFTransform_ProcessInput(e->mft, e->in_id, s, 0);
			e->need--;
			if (SUCCEEDED(hr))
				hr = events(e, 0);
		}
	} else {
		hr = IMFTransform_ProcessInput(e->mft, e->in_id, s, 0);
		while (SUCCEEDED(hr)) {
			HRESULT p = pull(e);
			if (p == MF_E_TRANSFORM_NEED_MORE_INPUT)
				break;
			if (FAILED(p)) {
				hr = p;
				break;
			}
		}
	}
	IMFSample_Release(s);
	return SUCCEEDED(hr) ? 0 : (int)hr;
}

static void mf_close(mf_enc *e) {
	stop(e);
	free(e->seq);
	free(e);
}

static const char *mf_name(mf_enc *e) { return e->name; }
static int mf_is_hardware(mf_enc *e) { return e->hardware; }
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"runtime/cgo"
	"unsafe"
)

// mfEncoder is H.264 from Media Foundation, Windows' own encoders: the
// graphics card's (Intel Quick Sync, NVIDIA NVENC, AMD AMF, through their
// drivers' Media Foundation encoders) or Windows' software one. Nothing to
// install, and the encoder's licensing comes with the system. Media
// Foundation objects belong to the thread that made them, so one goroutine
// locked to its OS thread owns the encoder and the frames go to it.
type mfEncoder struct {
	work     chan mfJob
	done     chan struct{}
	frameSz  int
	name     string
	hardware bool
}

type mfJob struct {
	i420 []byte
	key  bool
	stop bool
	err  chan error
}

//export goMFFrame
func goMFFrame(handle C.uintptr_t, data unsafe.Pointer, n C.int) {
	onFrame := cgo.Handle(handle).Value().(func([]byte))
	onFrame(C.GoBytes(data, n))
}

// newMF starts a Media Foundation encoder (hardware only, or with Windows'
// software encoder when hardware is false); onFrame gets each access unit
// (Annex B, with an access unit delimiter, and SPS and PPS before each
// keyframe) from the encoder's goroutine.
func newMF(cfg Config, gop int, hardware bool, onFrame func([]byte)) (*mfEncoder, error) {
	m := &mfEncoder{work: make(chan mfJob), done: make(chan struct{}), frameSz: cfg.Width*cfg.Height + 2*(cfg.Width/2)*(cfg.Height/2)}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(m.done)
		if st := C.mf_init(); st != 0 {
			ready <- fmt.Errorf("h264: Media Foundation did not start (0x%08x)", uint32(st))
			return
		}
		defer C.mf_done()
		h := cgo.NewHandle(onFrame)
		defer h.Delete()
		hw := C.int(0)
		if hardware {
			hw = 1
		}
		var cerr C.int
		e := C.mf_open(C.int(cfg.Width), C.int(cfg.Height), C.int(cfg.FPS), C.int(cfg.BitrateKbps), C.int(gop), hw, C.uintptr_t(h), &cerr)
		if e == nil {
			if cerr == -2 {
				ready <- errors.New("h264: no Media Foundation H.264 encoder on this computer")
			} else {
				ready <- fmt.Errorf("h264: Media Foundation encoder failed (0x%08x)", uint32(cerr))
			}
			return
		}
		m.name = C.GoString(C.mf_name(e))
		m.hardware = C.mf_is_hardware(e) != 0
		ready <- nil
		for j := range m.work {
			if j.stop {
				C.mf_close(e)
				j.err <- nil
				return
			}
			k := C.int(0)
			if j.key {
				k = 1
			}
			var err error
			if st := C.mf_encode(e, (*C.uint8_t)(unsafe.Pointer(&j.i420[0])), k); st != 0 {
				err = fmt.Errorf("h264: Media Foundation encode failed (0x%08x)", uint32(st))
			}
			j.err <- err
		}
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	return m, nil
}

func (m *mfEncoder) write(i420 []byte, key bool) error {
	if len(i420) < m.frameSz {
		return fmt.Errorf("h264: frame is %d bytes, want %d", len(i420), m.frameSz)
	}
	j := mfJob{i420: i420, key: key, err: make(chan error, 1)}
	select {
	case m.work <- j:
	case <-m.done:
		return errors.New("h264: closed")
	}
	return <-j.err
}

func (m *mfEncoder) close() {
	j := mfJob{stop: true, err: make(chan error, 1)}
	select {
	case m.work <- j:
		<-j.err
	case <-m.done:
	}
	<-m.done
}

// MFHardware is the name of the computer's first hardware H.264 encoder
// that Media Foundation lists ("" when there is none: then rooms keep VP8).
func MFHardware() string {
	out := make(chan string, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if C.mf_init() != 0 {
			out <- ""
			return
		}
		defer C.mf_done()
		var buf [128]C.char
		C.mf_hardware_name(&buf[0], C.int(len(buf)))
		out <- C.GoString(&buf[0])
	}()
	return <-out
}

// hasMF is whether this build can call Media Foundation.
const hasMF = true
