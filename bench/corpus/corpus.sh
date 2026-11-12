#!/bin/sh
# corpus.sh DIR [small|large]: writes the benchmark corpus (large, the default) or the
# library's committed test fixtures (small) into DIR. The metadata is written by exiftool, an
# implementation independent of this package. Needs exiftool, cwebp and sips (macOS) or
# ImageMagick's convert for the TIFF.
set -eu
dir=$1
size=${2:-large}
here=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$dir"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
(cd "$here" && go run gen.go "$work")

camera() { # file byteorder: Sydney, below sea level, orientation 6, offset +10:00
	exiftool -q -overwrite_original -n -ExifByteOrder="$2" \
		-Make=Canon -Model="Canon EOS R5" -Software="exif-corpus 1.0" -Artist="Test Author" \
		-Orientation=6 -XResolution=72 -YResolution=72 -ResolutionUnit=2 \
		-DateTimeOriginal="2024:07:14 18:30:05" -OffsetTimeOriginal="+10:00" -SubSecTimeOriginal=042 \
		-CreateDate="2024:07:14 18:30:05" -ModifyDate="2024:07:15 09:00:00" \
		-ExposureTime=0.005 -FNumber=2.8 -ISO=200 -FocalLength=50 -LensModel="RF50mm F1.8 STM" \
		-ExifImageWidth=4032 -ExifImageHeight=3024 \
		-GPSLatitude=33.8688 -GPSLatitudeRef=S -GPSLongitude=151.2093 -GPSLongitudeRef=E \
		-GPSAltitude=12.5 -GPSAltitudeRef=1 -GPSStatus=A \
		-XMP-dc:Creator="Test Author" "$1"
}
phone() { # file byteorder: Madrid, above sea level, orientation 8, offset -05:00
	exiftool -q -overwrite_original -n -ExifByteOrder="$2" \
		-Make=Apple -Model="iPhone 15 Pro" -Orientation=8 \
		-DateTimeOriginal="2023:12:31 23:59:59" -OffsetTimeOriginal="-05:00" \
		-ExifImageWidth=4032 -ExifImageHeight=3024 \
		-GPSLatitude=40.416775 -GPSLatitudeRef=N -GPSLongitude=3.70379 -GPSLongitudeRef=W \
		-GPSAltitude=657 -GPSAltitudeRef=0 "$1"
}

cp "$work/base-$size.jpg" "$dir/camera.jpg"
camera "$dir/camera.jpg" II
exiftool -q -overwrite_original "-ThumbnailImage<=$work/thumb.jpg" "$dir/camera.jpg"
cp "$work/base-$size.jpg" "$dir/phone.jpg"
phone "$dir/phone.jpg" MM
cp "$work/base-$size.png" "$dir/camera.png"
camera "$dir/camera.png" MM
cwebp -quiet -q 80 "$work/base-$size.png" -o "$dir/camera.webp"
camera "$dir/camera.webp" II
if command -v sips >/dev/null; then
	sips -s format tiff "$work/base-$size.png" --out "$dir/camera.tif" >/dev/null
else
	convert "$work/base-$size.png" "$dir/camera.tif"
fi
camera "$dir/camera.tif" MM
cp "$work/base-$size.jpg" "$dir/plain.jpg"
ls -l "$dir"
