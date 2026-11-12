package exif

// Error is the type of every error this package returns. Its values are constants, so returning
// one never allocates, and callers compare them with errors.Is or ==.
type Error string

func (e Error) Error() string { return string(e) }

const (
	// ErrUnknownFormat reports input that is not a JPEG, TIFF, PNG, WebP or raw EXIF block.
	ErrUnknownFormat Error = "exif: unknown image format"
	// ErrNoExif reports a well-formed container that carries no EXIF metadata.
	ErrNoExif Error = "exif: no EXIF metadata"
	// ErrMalformed reports a structure that is truncated, points outside its data or breaks
	// the format's rules.
	ErrMalformed Error = "exif: malformed data"
	// ErrTooLarge reports metadata or input beyond the decoder's limits.
	ErrTooLarge Error = "exif: data exceeds the size limit"
	// ErrType reports a tag read as a type it does not hold, such as a RATIONAL read as text.
	ErrType Error = "exif: tag has another type"
	// ErrIndex reports a value index outside a tag's count.
	ErrIndex Error = "exif: value index out of range"
	// ErrZeroDenominator reports a rational whose denominator is zero.
	ErrZeroDenominator Error = "exif: rational with a zero denominator"
)
