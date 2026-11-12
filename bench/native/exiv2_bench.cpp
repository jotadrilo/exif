// exiv2_bench MODE FILE TASK SECONDS RSSRUNS: the runner's protocol for exiv2. See
// libexif_bench.c.
#include <exiv2/exiv2.hpp>
#include <chrono>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <fstream>
#include <algorithm>
#include <atomic>
#include <iterator>
#include <thread>
#include <sys/resource.h>
#include <vector>

struct result {
	int orientation = 1;
	bool has_gps = false, has_alt = false;
	double lat = 0, lon = 0, alt = 0;
	std::string taken;
};

static bool degrees(const Exiv2::ExifData &ed, const char *key, const char *refkey, double &out) {
	auto e = ed.findKey(Exiv2::ExifKey(key));
	auto r = ed.findKey(Exiv2::ExifKey(refkey));
	if (e == ed.end() || r == ed.end() || e->count() < 1) return false;
	double d = 0, scale = 1;
	for (size_t i = 0; i < e->count() && i < 3; i++) {
		auto q = e->toRational(i);
		if (q.second == 0) return false;
		d += double(q.first) / q.second / scale;
		scale *= 60;
	}
	auto ref = r->toString();
	if (!ref.empty() && (ref[0] == 'S' || ref[0] == 'W')) d = -d;
	out = d;
	return true;
}

static void parse(Exiv2::Image::UniquePtr image, bool all, result &res) {
	image->readMetadata();
	const Exiv2::ExifData &ed = image->exifData();
	res = result();
	auto o = ed.findKey(Exiv2::ExifKey("Exif.Image.Orientation"));
	if (o != ed.end() && o->count() > 0) {
		auto v = o->toInt64(0);
		if (v >= 1 && v <= 8) res.orientation = int(v);
	}
	if (!all) return;
	res.has_gps = degrees(ed, "Exif.GPSInfo.GPSLatitude", "Exif.GPSInfo.GPSLatitudeRef", res.lat) &&
	              degrees(ed, "Exif.GPSInfo.GPSLongitude", "Exif.GPSInfo.GPSLongitudeRef", res.lon);
	auto a = ed.findKey(Exiv2::ExifKey("Exif.GPSInfo.GPSAltitude"));
	if (a != ed.end() && a->count() > 0) {
		auto q = a->toRational(0);
		if (q.second) {
			res.has_alt = true;
			res.alt = double(q.first) / q.second;
			auto ar = ed.findKey(Exiv2::ExifKey("Exif.GPSInfo.GPSAltitudeRef"));
			if (ar != ed.end() && ar->toInt64(0) == 1) res.alt = -res.alt;
		}
	}
	auto t = ed.findKey(Exiv2::ExifKey("Exif.Photo.DateTimeOriginal"));
	if (t != ed.end()) res.taken = t->toString();
}

int main(int argc, char **argv) {
	if (argc < 6) {
		std::fprintf(stderr, "usage: %s time|rss|values FILE TASK SECONDS RSSRUNS\n", argv[0]);
		return 2;
	}
	Exiv2::LogMsg::setLevel(Exiv2::LogMsg::mute);
	Exiv2::XmpParser::initialize();
	std::string mode = argv[1], file = argv[2];
	bool all = std::strcmp(argv[3], "all") == 0;
	double seconds = std::atof(argv[4]);
	int runs = std::atoi(argv[5]);
	result r;
	try {
		if (mode == "rss") {
			for (int i = 0; i < runs; i++) parse(Exiv2::ImageFactory::open(file), all, r);
			struct rusage ru;
			getrusage(RUSAGE_SELF, &ru);
#ifdef __APPLE__
			long kb = ru.ru_maxrss / 1024;
#else
			long kb = ru.ru_maxrss;
#endif
			std::printf("{\"maxrss_kb\":%ld}\n", kb);
			return 0;
		}
		std::ifstream in(file, std::ios::binary);
		std::vector<Exiv2::byte> b((std::istreambuf_iterator<char>(in)), std::istreambuf_iterator<char>());
		if (mode == "latency") {
			using clk = std::chrono::steady_clock;
			parse(Exiv2::ImageFactory::open(b.data(), b.size()), all, r);
			std::vector<double> ov, s;
			for (int i = 0; i < 1000; i++) {
				auto t0 = clk::now();
				ov.push_back(std::chrono::duration<double, std::nano>(clk::now() - t0).count());
			}
			std::sort(ov.begin(), ov.end());
			double clock = ov[500];
			auto deadline = clk::now() + std::chrono::duration<double>(seconds);
			while (s.size() < 200000 && clk::now() < deadline) {
				auto t0 = clk::now();
				parse(Exiv2::ImageFactory::open(b.data(), b.size()), all, r);
				s.push_back(std::max(0.0, std::chrono::duration<double, std::nano>(clk::now() - t0).count() - clock));
			}
			std::sort(s.begin(), s.end());
			size_t k = s.size();
			std::printf("{\"p50\":%.1f,\"p95\":%.1f,\"p99\":%.1f}\n", s[k / 2], s[size_t(k * 0.95)], s[size_t(k * 0.99)]);
			return 0;
		}
		if (mode == "throughput") {
			int nw = argc > 6 ? std::atoi(argv[6]) : 1;
			parse(Exiv2::ImageFactory::open(b.data(), b.size()), all, r);
			std::atomic<long> total{0};
			auto start = std::chrono::steady_clock::now();
			auto deadline = start + std::chrono::duration<double>(seconds);
			std::vector<std::thread> th;
			for (int i = 0; i < nw; i++)
				th.emplace_back([&] {
					result local;
					long ops = 0;
					while (std::chrono::steady_clock::now() < deadline) {
						for (int j = 0; j < 16; j++) parse(Exiv2::ImageFactory::open(b.data(), b.size()), all, local);
						ops += 16;
					}
					total += ops;
				});
			for (auto &t : th) t.join();
			double el = std::chrono::duration<double>(std::chrono::steady_clock::now() - start).count();
			std::printf("{\"ops_s\":%.1f}\n", total.load() / el);
			return 0;
		}
		if (mode == "values") {
			parse(Exiv2::ImageFactory::open(b.data(), b.size()), true, r);
			std::printf("{\"orientation\":%d,\"has_gps\":%s,\"lat\":%.9f,\"lon\":%.9f,\"has_alt\":%s,\"alt\":%.6f,\"taken\":\"%s\"}\n",
			            r.orientation, r.has_gps ? "true" : "false", r.lat, r.lon, r.has_alt ? "true" : "false", r.alt,
			            r.taken.c_str());
			return 0;
		}
		for (long iters = 1;; iters *= 2) {
			auto start = std::chrono::steady_clock::now();
			for (long i = 0; i < iters; i++) parse(Exiv2::ImageFactory::open(b.data(), b.size()), all, r);
			double el = std::chrono::duration<double>(std::chrono::steady_clock::now() - start).count();
			if (el >= seconds) {
				std::printf("{\"ns_op\":%.1f,\"allocs_op\":-1,\"bytes_op\":-1}\n", el * 1e9 / iters);
				return 0;
			}
		}
	} catch (const std::exception &e) {
		if (mode == "values") {
			std::printf("{\"orientation\":1,\"err\":\"%s\"}\n", "exiv2 error");
			return 0;
		}
		std::fprintf(stderr, "%s\n", e.what());
		return 1;
	}
}
