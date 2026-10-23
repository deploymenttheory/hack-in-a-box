package media_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"

	"github.com/google/uuid"

	"github.com/deploymenttheory/hack-in-a-box/internal/access"
	"github.com/deploymenttheory/hack-in-a-box/internal/files"
	"github.com/deploymenttheory/hack-in-a-box/internal/media"
)

func TestPhotographDecodeBoundsRejectInvalidBytesBeforeStorage(t *testing.T) {
	// A legal PNG header declaring excessive dimensions must be rejected before
	// allocating a decoded image or touching a database/storage provider.
	header := make([]byte, 13)
	binary.BigEndian.PutUint32(header[0:4], 100000)
	binary.BigEndian.PutUint32(header[4:8], 100000)
	header[8], header[9] = 8, 6
	var huge bytes.Buffer
	huge.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	_ = binary.Write(&huge, binary.BigEndian, uint32(len(header)))
	chunk := append([]byte("IHDR"), header...)
	huge.Write(chunk)
	_ = binary.Write(&huge, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	for _, data := range [][]byte{[]byte("Not an image"), photo()[:20], huge.Bytes()} {
		_, err := (media.Service{}).CollectPhoto(
			t.Context(),
			media.Upload{
				ID:       uuid.New(),
				Metadata: media.Metadata{Kind: "photo", Title: "Synthetic rejected photograph"},
				Reason:   "Decode must precede storage",
				File:     files.Upload{MediaType: "image/png", Content: data},
			},
		)
		if !errors.Is(err, access.ErrInvalid) {
			t.Fatal("invalid/oversized photograph reached native storage", err)
		}
	}
}
