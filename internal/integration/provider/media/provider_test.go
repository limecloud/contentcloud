package mediapipeline

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
)

func TestFakeProviderVideoMatchesFixtureDuration(t *testing.T) {
	download := Download{Body: fakeMP4("fixture"), MediaType: "video/mp4", FileName: "generated-take.mp4"}
	technical, err := ValidateDownload(download, 10<<20)
	if err != nil {
		t.Fatalf("fixture video failed container validation: %v", err)
	}
	if technical["validated"] != true {
		t.Fatalf("fixture video is not marked as validated: %#v", technical)
	}
	duration, err := mp4DurationSeconds(download.Body)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(duration-26) > 0.001 {
		t.Fatalf("fixture video duration=%v, want 26 seconds", duration)
	}
}

func TestValidateDownloadReaderBoundsAndHashesStream(t *testing.T) {
	body := fakeMP4("stream")
	validated, err := ValidateDownloadReader(bytes.NewReader(body), "video/mp4", int64(len(body)))
	if err != nil {
		t.Fatalf("streaming MP4 validation failed: %v", err)
	}
	if validated.ByteSize != int64(len(body)) || validated.SHA256 != SHA256(body) || validated.Technical["validated"] != true {
		t.Fatalf("unexpected streaming validation: %#v", validated)
	}
	if _, err := ValidateDownloadReader(bytes.NewReader(append(body, 0)), "video/mp4", int64(len(body))); err == nil {
		t.Fatal("streaming validator accepted a response over the configured limit")
	}
}

func mp4DurationSeconds(body []byte) (float64, error) {
	moov, err := mp4BoxPayload(body, "moov")
	if err != nil {
		return 0, err
	}
	mvhd, err := mp4BoxPayload(moov, "mvhd")
	if err != nil {
		return 0, err
	}
	if len(mvhd) < 20 {
		return 0, fmt.Errorf("mvhd box is incomplete")
	}
	var timescale uint32
	var duration uint64
	switch mvhd[0] {
	case 0:
		timescale = binary.BigEndian.Uint32(mvhd[12:16])
		duration = uint64(binary.BigEndian.Uint32(mvhd[16:20]))
	case 1:
		if len(mvhd) < 32 {
			return 0, fmt.Errorf("version 1 mvhd box is incomplete")
		}
		timescale = binary.BigEndian.Uint32(mvhd[20:24])
		duration = binary.BigEndian.Uint64(mvhd[24:32])
	default:
		return 0, fmt.Errorf("unsupported mvhd version %d", mvhd[0])
	}
	if timescale == 0 {
		return 0, fmt.Errorf("mvhd timescale is zero")
	}
	return float64(duration) / float64(timescale), nil
}

func mp4BoxPayload(body []byte, target string) ([]byte, error) {
	for offset := 0; offset+8 <= len(body); {
		size32 := binary.BigEndian.Uint32(body[offset : offset+4])
		headerSize := uint64(8)
		var size uint64
		switch size32 {
		case 0:
			size = uint64(len(body) - offset)
		case 1:
			if offset+16 > len(body) {
				return nil, fmt.Errorf("extended MP4 box header is incomplete")
			}
			headerSize = 16
			size = binary.BigEndian.Uint64(body[offset+8 : offset+16])
		default:
			size = uint64(size32)
		}
		if size < headerSize || size > uint64(len(body)-offset) {
			return nil, fmt.Errorf("invalid MP4 box size")
		}
		if string(body[offset+4:offset+8]) == target {
			return body[offset+int(headerSize) : offset+int(size)], nil
		}
		offset += int(size)
	}
	return nil, fmt.Errorf("MP4 box %q not found", target)
}
