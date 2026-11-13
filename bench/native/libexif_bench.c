// libexif_bench MODE FILE TASK SECONDS RSSRUNS: the runner's protocol for libexif.
// MODE time: parse FILE from memory in a loop for SECONDS; rss: parse it from its path
// RSSRUNS times; values: print what it reads. libexif finds EXIF in JPEG files and raw EXIF
// blocks only.
#include <libexif/exif-data.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/resource.h>
#include <time.h>

typedef struct {
	int orientation, has_gps, has_alt;
	double lat, lon, alt;
	char taken[32];
} result;

// now reads a nanosecond clock: on macOS, CLOCK_MONOTONIC ticks only every microsecond.
static double now(void) {
#ifdef __APPLE__
	return clock_gettime_nsec_np(CLOCK_UPTIME_RAW) / 1e9;
#else
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return ts.tv_sec + ts.tv_nsec / 1e9;
#endif
}

static int degrees(ExifData *ed, ExifTag tag, ExifTag reftag, double *out) {
	ExifEntry *e = exif_content_get_entry(ed->ifd[EXIF_IFD_GPS], tag);
	ExifEntry *r = exif_content_get_entry(ed->ifd[EXIF_IFD_GPS], reftag);
	if (!e || !r || e->format != EXIF_FORMAT_RATIONAL || e->components < 1 || r->size < 1) return 0;
	ExifByteOrder o = exif_data_get_byte_order(ed);
	double d = 0, scale = 1;
	for (unsigned i = 0; i < e->components && i < 3; i++) {
		ExifRational q = exif_get_rational(e->data + 8 * i, o);
		if (q.denominator == 0) return 0;
		d += (double)q.numerator / q.denominator / scale;
		scale *= 60;
	}
	if (r->data[0] == 'S' || r->data[0] == 'W') d = -d;
	*out = d;
	return 1;
}

static int parse(ExifData *ed, int all, result *res) {
	if (!ed) return -1;
	memset(res, 0, sizeof *res);
	res->orientation = 1;
	ExifByteOrder o = exif_data_get_byte_order(ed);
	ExifEntry *e = exif_content_get_entry(ed->ifd[EXIF_IFD_0], EXIF_TAG_ORIENTATION);
	if (e && e->format == EXIF_FORMAT_SHORT && e->components > 0) {
		int v = exif_get_short(e->data, o);
		if (v >= 1 && v <= 8) res->orientation = v;
	}
	if (all) {
		if (degrees(ed, EXIF_TAG_GPS_LATITUDE, EXIF_TAG_GPS_LATITUDE_REF, &res->lat) &&
		    degrees(ed, EXIF_TAG_GPS_LONGITUDE, EXIF_TAG_GPS_LONGITUDE_REF, &res->lon))
			res->has_gps = 1;
		ExifEntry *a = exif_content_get_entry(ed->ifd[EXIF_IFD_GPS], EXIF_TAG_GPS_ALTITUDE);
		if (a && a->format == EXIF_FORMAT_RATIONAL && a->components > 0) {
			ExifRational q = exif_get_rational(a->data, o);
			if (q.denominator) {
				res->has_alt = 1;
				res->alt = (double)q.numerator / q.denominator;
				ExifEntry *ar = exif_content_get_entry(ed->ifd[EXIF_IFD_GPS], EXIF_TAG_GPS_ALTITUDE_REF);
				if (ar && ar->size > 0 && ar->data[0] == 1) res->alt = -res->alt;
			}
		}
		ExifEntry *t = exif_content_get_entry(ed->ifd[EXIF_IFD_EXIF], EXIF_TAG_DATE_TIME_ORIGINAL);
		if (t && t->format == EXIF_FORMAT_ASCII && t->size >= 19) {
			memcpy(res->taken, t->data, 19);
			res->taken[19] = 0;
		}
	}
	exif_data_unref(ed);
	return 0;
}

static unsigned char *slurp(const char *path, size_t *n) {
	FILE *f = fopen(path, "rb");
	if (!f) { perror(path); exit(1); }
	fseek(f, 0, SEEK_END);
	*n = ftell(f);
	fseek(f, 0, SEEK_SET);
	unsigned char *b = malloc(*n);
	if (fread(b, 1, *n, f) != *n) { perror("read"); exit(1); }
	fclose(f);
	return b;
}

typedef struct {
	unsigned char *b;
	size_t n;
	int all;
	double deadline;
	long ops;
} worker;

static void *work(void *arg) {
	worker *w = arg;
	result r;
	while (now() < w->deadline) {
		for (int i = 0; i < 16; i++) parse(exif_data_new_from_data(w->b, w->n), w->all, &r);
		w->ops += 16;
	}
	return NULL;
}

static int cmpd(const void *a, const void *b) {
	double x = *(const double *)a, y = *(const double *)b;
	return x < y ? -1 : x > y;
}

int main(int argc, char **argv) {
	if (argc < 6) { fprintf(stderr, "usage: %s time|rss|values|throughput FILE TASK SECONDS RSSRUNS [WORKERS]\n", argv[0]); return 2; }
	const char *mode = argv[1], *file = argv[2];
	int all = strcmp(argv[3], "all") == 0;
	double seconds = atof(argv[4]);
	int runs = atoi(argv[5]);
	result r;
	if (!strcmp(mode, "values")) {
		size_t n;
		unsigned char *b = slurp(file, &n);
		ExifData *ed = exif_data_new_from_data(b, n);
		int has = ed && (ed->ifd[EXIF_IFD_0]->count || ed->ifd[EXIF_IFD_EXIF]->count);
		parse(ed, 1, &r);
		printf("{\"orientation\":%d,\"has_gps\":%s,\"lat\":%.9f,\"lon\":%.9f,\"has_alt\":%s,\"alt\":%.6f,\"taken\":\"%s\"%s}\n",
		       r.orientation, r.has_gps ? "true" : "false", r.lat, r.lon, r.has_alt ? "true" : "false", r.alt, r.taken,
		       has ? "" : ",\"err\":\"no EXIF found\"");
		return 0;
	}
	if (!strcmp(mode, "latency")) {
		// Each parse timed alone, less the cost of reading the clock twice.
		size_t n;
		unsigned char *b = slurp(file, &n);
		parse(exif_data_new_from_data(b, n), all, &r);
		enum { MAX = 200000 };
		double *s = malloc(MAX * sizeof *s), ov[1000];
		for (int i = 0; i < 1000; i++) {
			double t0 = now();
			ov[i] = (now() - t0) * 1e9;
		}
		qsort(ov, 1000, sizeof *ov, cmpd);
		double clock = ov[500], deadline = now() + seconds;
		int k = 0;
		while (k < MAX && now() < deadline) {
			double t0 = now();
			parse(exif_data_new_from_data(b, n), all, &r);
			double d = (now() - t0) * 1e9 - clock;
			s[k++] = d > 0 ? d : 0;
		}
		qsort(s, k, sizeof *s, cmpd);
		printf("{\"p50\":%.1f,\"p95\":%.1f,\"p99\":%.1f}\n", s[k / 2], s[(int)(k * 0.95)], s[(int)(k * 0.99)]);
		return 0;
	}
	if (!strcmp(mode, "throughput")) {
		int nw = argc > 6 ? atoi(argv[6]) : 1;
		size_t n;
		unsigned char *b = slurp(file, &n);
		parse(exif_data_new_from_data(b, n), all, &r);
		pthread_t th[64];
		worker ws[64];
		if (nw < 1 || nw > 64) nw = 1;
		double start = now();
		for (int i = 0; i < nw; i++) {
			ws[i] = (worker){b, n, all, start + seconds, 0};
			pthread_create(&th[i], NULL, work, &ws[i]);
		}
		long total = 0;
		for (int i = 0; i < nw; i++) {
			pthread_join(th[i], NULL);
			total += ws[i].ops;
		}
		printf("{\"ops_s\":%.1f}\n", total / (now() - start));
		return 0;
	}
	if (!strcmp(mode, "time")) {
		size_t n;
		unsigned char *b = slurp(file, &n);
		for (long iters = 1;; iters *= 2) {
			double start = now();
			for (long i = 0; i < iters; i++) parse(exif_data_new_from_data(b, n), all, &r);
			double el = now() - start;
			if (el >= seconds) {
				printf("{\"ns_op\":%.1f,\"allocs_op\":-1,\"bytes_op\":-1}\n", el * 1e9 / iters);
				return 0;
			}
		}
	}
	for (int i = 0; i < runs; i++) parse(exif_data_new_from_file(file), all, &r);
	struct rusage ru;
	getrusage(RUSAGE_SELF, &ru);
#ifdef __APPLE__
	long kb = ru.ru_maxrss / 1024;
#else
	long kb = ru.ru_maxrss;
#endif
	printf("{\"maxrss_kb\":%ld}\n", kb);
	return 0;
}
