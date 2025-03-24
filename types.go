package umpparser

type UMPData struct {
	MediaHeader *UMPMediaHeader // MediaHeader is part 20 of the UMP data
	Media       []byte          // Media is the raw media data, part 21 of the UMP data
}

// MediaHeader is a struct that represents part 20 of the UMP data
type UMPMediaHeader struct {
	VideoID string // VideoID is the unique identifier for the video, field 2
	ITag    int32  // ITag is the identifier for the video format, field 3
	Lmt     int64  // Lmt is the last modified time of the video, field 4
}
