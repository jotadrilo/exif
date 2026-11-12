package exif

// IFD names an image file directory of the EXIF structure.
type IFD uint8

// The directories this package reads. IFD0 describes the main image, IFD1 its thumbnail.
const (
	IFD0 IFD = iota
	ExifIFD
	GPSIFD
	InteropIFD
	IFD1
)

func (d IFD) String() string {
	switch d {
	case IFD0:
		return "IFD0"
	case ExifIFD:
		return "ExifIFD"
	case GPSIFD:
		return "GPSIFD"
	case InteropIFD:
		return "InteropIFD"
	case IFD1:
		return "IFD1"
	}
	return "IFD(?)"
}

// Type is a TIFF field type.
type Type uint16

// The TIFF field types. Long8, SLong8 and IFD8 come from BigTIFF and are read as integers.
const (
	Byte      Type = 1
	ASCII     Type = 2
	Short     Type = 3
	Long      Type = 4
	Rational  Type = 5
	SByte     Type = 6
	Undefined Type = 7
	SShort    Type = 8
	SLong     Type = 9
	SRational Type = 10
	Float     Type = 11
	Double    Type = 12
	IFDType   Type = 13
	Long8     Type = 16
	SLong8    Type = 17
	IFD8      Type = 18
)

// typeSizes holds the bytes one value of each type takes.
var typeSizes = [...]uint8{Byte: 1, ASCII: 1, Short: 2, Long: 4, Rational: 8, SByte: 1, Undefined: 1,
	SShort: 2, SLong: 4, SRational: 8, Float: 4, Double: 8, IFDType: 4, Long8: 8, SLong8: 8, IFD8: 8}

// Size returns the bytes one value of the type takes, or 0 for an unknown type.
func (t Type) Size() int {
	if int(t) < len(typeSizes) {
		return int(typeSizes[t])
	}
	return 0
}

func (t Type) isInteger() bool {
	switch t {
	case Byte, Short, Long, SByte, SShort, SLong, IFDType, Long8, SLong8, IFD8:
		return true
	}
	return false
}

func (t Type) String() string {
	switch t {
	case Byte:
		return "BYTE"
	case ASCII:
		return "ASCII"
	case Short:
		return "SHORT"
	case Long:
		return "LONG"
	case Rational:
		return "RATIONAL"
	case SByte:
		return "SBYTE"
	case Undefined:
		return "UNDEFINED"
	case SShort:
		return "SSHORT"
	case SLong:
		return "SLONG"
	case SRational:
		return "SRATIONAL"
	case Float:
		return "FLOAT"
	case Double:
		return "DOUBLE"
	case IFDType:
		return "IFD"
	case Long8:
		return "LONG8"
	case SLong8:
		return "SLONG8"
	case IFD8:
		return "IFD8"
	}
	return "UNKNOWN"
}

// Tag IDs of IFD0 and IFD1.
const (
	TagImageWidth                  uint16 = 0x0100
	TagImageLength                 uint16 = 0x0101
	TagImageDescription            uint16 = 0x010E
	TagMake                        uint16 = 0x010F
	TagModel                       uint16 = 0x0110
	TagOrientation                 uint16 = 0x0112
	TagXResolution                 uint16 = 0x011A
	TagYResolution                 uint16 = 0x011B
	TagResolutionUnit              uint16 = 0x0128
	TagSoftware                    uint16 = 0x0131
	TagDateTime                    uint16 = 0x0132
	TagArtist                      uint16 = 0x013B
	TagJPEGInterchangeFormat       uint16 = 0x0201
	TagJPEGInterchangeFormatLength uint16 = 0x0202
	TagCopyright                   uint16 = 0x8298
	TagExifIFDPointer              uint16 = 0x8769
	TagGPSIFDPointer               uint16 = 0x8825
)

// Tag IDs of the Exif IFD.
const (
	TagExposureTime        uint16 = 0x829A
	TagFNumber             uint16 = 0x829D
	TagISOSpeedRatings     uint16 = 0x8827
	TagExifVersion         uint16 = 0x9000
	TagDateTimeOriginal    uint16 = 0x9003
	TagDateTimeDigitized   uint16 = 0x9004
	TagOffsetTime          uint16 = 0x9010
	TagOffsetTimeOriginal  uint16 = 0x9011
	TagOffsetTimeDigitized uint16 = 0x9012
	TagFocalLength         uint16 = 0x920A
	TagMakerNote           uint16 = 0x927C
	TagUserComment         uint16 = 0x9286
	TagSubSecTime          uint16 = 0x9290
	TagSubSecTimeOriginal  uint16 = 0x9291
	TagPixelXDimension     uint16 = 0xA002
	TagPixelYDimension     uint16 = 0xA003
	TagInteropIFDPointer   uint16 = 0xA005
	TagImageUniqueID       uint16 = 0xA420
	TagCameraOwnerName     uint16 = 0xA430
	TagBodySerialNumber    uint16 = 0xA431
	TagLensMake            uint16 = 0xA433
	TagLensModel           uint16 = 0xA434
)

// Tag IDs of the GPS IFD.
const (
	TagGPSVersionID    uint16 = 0x0000
	TagGPSLatitudeRef  uint16 = 0x0001
	TagGPSLatitude     uint16 = 0x0002
	TagGPSLongitudeRef uint16 = 0x0003
	TagGPSLongitude    uint16 = 0x0004
	TagGPSAltitudeRef  uint16 = 0x0005
	TagGPSAltitude     uint16 = 0x0006
	TagGPSTimeStamp    uint16 = 0x0007
	TagGPSStatus       uint16 = 0x0009
	TagGPSImgDirection uint16 = 0x0011
	TagGPSDateStamp    uint16 = 0x001D
)
