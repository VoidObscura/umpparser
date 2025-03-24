package umpparser

import (
	"bytes"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
)

// getVarIntSize returns the number of bytes used by the varint given the first byte.
// According to the spec, the first 5 bits of the first byte indicate the size:
//   - if the top bit is 0: 1 byte
//   - if the top bit is 1 but the next is 0: 2 bytes
//   - if the top two bits are 1 but the next is 0: 3 bytes
//   - if the top three bits are 1 but the next is 0: 4 bytes
//   - if the top four bits are 1 but the next is 0: 5 bytes
//
// If all five top bits are set, the varint is invalid.
func getVarIntSize(b byte) int {
	for shift := 1; shift <= 5; shift++ {
		if (b & (0x80 >> (shift - 1))) == 0 {
			return shift
		}
	}
	return 0 // indicates invalid varint
}

// readVarInt reads a variable‑length integer from data starting at *offset.
// It follows the spec: for sizes 2–4, the remainder of the first byte is used as the
// low-order bits of the integer. For 5‑byte integers the first byte’s lower bits are ignored.
// This implementation interprets the continuation bytes in little‑endian order.
func readVarInt(data []byte, offset *int) (int, error) {
	start := *offset
	if start >= len(data) {
		return 0, errors.New("no data available")
	}
	first := data[start]
	size := getVarIntSize(first)
	if size == 0 {
		return 0, fmt.Errorf("invalid varint at pos %d: all top 5 bits set", start)
	}
	// Check if we have enough bytes
	if start+size > len(data) {
		return 0, errors.New("incomplete varint")
	}
	var value int
	switch size {
	case 1:
		// 1-byte varint: value is the byte itself.
		value = int(first)
	case 2:
		// 2-byte varint: lower 6 bits of first byte, then next byte shifted left 6.
		value = int(first&0x3F) | (int(data[start+1]) << 6)
	case 3:
		// 3-byte varint: lower 5 bits of first byte,
		// second byte shifted left 5, third byte shifted left (5+8)=13.
		value = int(first&0x1F) | (int(data[start+1]) << 5) | (int(data[start+2]) << 13)
	case 4:
		// 4-byte varint: lower 4 bits of first byte,
		// then shifts of 4, 12, and 20.
		value = int(first&0x0F) | (int(data[start+1]) << 4) | (int(data[start+2]) << 12) | (int(data[start+3]) << 20)
	case 5:
		// 5-byte varint: per spec, ignore the lower 3 bits of first byte,
		// then a 32-bit little-endian integer from the next four bytes.
		value = int(data[start+1]) | (int(data[start+2]) << 8) | (int(data[start+3]) << 16) | (int(data[start+4]) << 24)
	}
	*offset += size
	return value, nil
}

// stateMachineParser processes the accumulated buffer using the UMP state machine.
// It follows these steps:
//  1. While there is data, attempt to read a complete part header (type and payload length).
//  2. If there isn’t enough data for the header or the full payload, break (to wait for more data).
//  3. Otherwise, extract the payload and process the part.
//     For media parts (type 21), if this is the very first media block, skip an initial null byte.
//  4. Remove the consumed bytes from the buffer and repeat.
func stateMachineParser(buf *bytes.Buffer, mediaAcc *bytes.Buffer, firstMedia *bool) (*UMPMediaHeader, error) {
	data := buf.Bytes()
	offset := 0
	var umpHeader *UMPMediaHeader
	for {
		// Step 1: If no data remains, break.
		if offset >= len(data) {
			break
		}
		// Step 2: Read part type.
		partType, err := readVarInt(data, &offset)
		if err != nil {
			// Not enough data for a complete header.
			break
		}
		// Read part payload length.
		partSize, err := readVarInt(data, &offset)
		if err != nil {
			// Incomplete header; reset offset to beginning of this part.
			offset -= 0 // (do nothing here)
			break
		}
		// Step 3: If part size is zero, it's a zero-length part.
		if partSize == 0 {
			// Nothing to do; continue with the next part.
			continue
		}
		// Step 4: Check if we have enough bytes remaining for the payload.
		if offset+partSize > len(data) {
			// Incomplete part; break out and wait for more data.
			break
		}
		// We have a complete part.
		payload := data[offset : offset+partSize]
		// Process the part based on its type.
		// (The spec says that current type numbers are below 128.)
		if partType == 20 {
			// Media header part.
			var protoHeader MediaHeader
			if err := proto.Unmarshal(payload, &protoHeader); err != nil {
				return nil, fmt.Errorf("failed to unmarshal media header: %w", err)
			}
			// Create the UMPMediaHeader struct from the proto message.
			umpHeader = &UMPMediaHeader{
				VideoID: protoHeader.GetVideoId(),
				ITag:    protoHeader.GetItag(),
				Lmt:     protoHeader.GetLmt(),
			}
		}
		if partType == 21 {
			// Media part.
			if !*firstMedia && len(payload) > 0 && payload[0] == 0 {
				// For the very first media part, skip the leading null byte.
				mediaAcc.Write(payload[1:])
			} else {
				mediaAcc.Write(payload)
			}
			*firstMedia = true
		}
		// (You can add handling for Part 22 (MEDIA_END) here if desired.)
		// Step 5: Increment offset by the payload size.
		offset += partSize
	}
	// Remove the consumed bytes from the buffer.
	buf.Next(offset)
	return umpHeader, nil
}

// ParseUMPChunks processes a slice of byte slices (chunks) containing UMP data.
// It accumulates the media data and extracts the media header from the first chunk.
// It returns a UMPData struct containing the media header and the accumulated media data.
// If no valid data is found, it returns an error.
// This function is useful for parsing UMP data that may be split across multiple chunks, such as
// multiple responses from a network stream or file read operations.
// It handles empty chunks gracefully and skips them.
func ParseUMPChunks(data [][]byte) (UMPData, error) {
	if len(data) == 0 {
		return UMPData{}, errors.New("no data to parse")
	}

	var mediaAcc bytes.Buffer
	var firstMedia bool
	var umpHeader *UMPMediaHeader
	buf := bytes.NewBuffer(nil)

	for _, chunk := range data {
		if len(chunk) == 0 {
			continue // skip empty chunks
		}
		buf.Write(chunk)
		h, err := stateMachineParser(buf, &mediaAcc, &firstMedia)
		if err != nil {
			return UMPData{}, fmt.Errorf("failed to parse UMP data: %w", err)
		}
		if h != nil {
			umpHeader = h // update the header if we found one
		}
	}

	return UMPData{
		MediaHeader: umpHeader,
		Media:       mediaAcc.Bytes(),
	}, nil
}

// ParseUMPChunks processes a slice of byte slices (chunks) containing UMP data.
// It accumulates the media data and extracts the media header from the first chunk.
// It returns a UMPData struct containing the media header and the accumulated media data.
// If no valid data is found, it returns an error.
// This function is useful for parsing UMP data that has already been joined into a single byte slice.
func ParseUMPFull(data []byte) (UMPData, error) {
	if len(data) == 0 {
		return UMPData{}, errors.New("no data to parse")
	}

	var mediaAcc bytes.Buffer
	var firstMedia bool
	var umpHeader *UMPMediaHeader
	buf := bytes.NewBuffer(data)

	for buf.Len() > 0 {
		h, err := stateMachineParser(buf, &mediaAcc, &firstMedia)
		if err != nil {
			return UMPData{}, fmt.Errorf("failed to parse UMP data: %w", err)
		}
		if h != nil {
			umpHeader = h // update the header if we found one
		}
	}

	return UMPData{
		MediaHeader: umpHeader,
		Media:       mediaAcc.Bytes(),
	}, nil
}
