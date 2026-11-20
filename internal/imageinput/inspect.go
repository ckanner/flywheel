package imageinput

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"slices"
	"strings"

	"golang.org/x/image/draw"
)

// Inspection is an explicit, read-only selection. It is not an approved packet
// or proof that any model has consumed the image. Only small PNG previews leave
// this boundary; the original remains in the selected repository.
type Inspection struct {
	References References `json:"references"`
	Previews   []Preview  `json:"previews"`
}

type Preview struct {
	Path     string `json:"path"`
	MIMEType string `json:"mimeType"`
	SHA256   string `json:"sha256"`
	Bytes    int    `json:"bytes"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	DataURL  string `json:"dataUrl"`
}

func Inspect(directory string, paths []string) (*Inspection, error) {
	view := &Inspection{References: References{Version: 1}, Previews: []Preview{}}
	for _, path := range paths {
		// Reuse the dispatch path/count validation before touching any file.
		view.References.Images = append(view.References.Images, Reference{Path: path, SHA256: strings.Repeat("0", 64), MediaType: "image/png"})
	}
	if err := Validate(&view.References); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	total := 0
	for index, ref := range view.References.Images {
		raw, err := readImage(root, ref.Path)
		if err != nil {
			return nil, fmt.Errorf("input image %q: %w", ref.Path, err)
		}
		total += len(raw)
		if total > MaxTotalBytes {
			return nil, fmt.Errorf("input images exceed the 8 MiB combined byte limit")
		}
		_, format, _ := image.DecodeConfig(bytes.NewReader(raw))
		mediaType := "image/" + format
		if !slices.Contains(mediaTypes, mediaType) {
			return nil, fmt.Errorf("input image %q is not a decodable image in PNG/JPEG/GIF/WebP format", ref.Path)
		}
		if err := validateRaster(raw, mediaType); err != nil {
			return nil, fmt.Errorf("input image %q: %w", ref.Path, err)
		}
		// Immutable bytes have already passed the complete decoder above.
		decoded, _, _ := image.Decode(bytes.NewReader(raw))
		bounds := decoded.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		longest := max(width, height, 160)
		preview := image.NewNRGBA(image.Rect(0, 0, max(1, width*160/longest), max(1, height*160/longest)))
		draw.ApproxBiLinear.Scale(preview, preview.Bounds(), decoded, bounds, draw.Src, nil)
		var encoded bytes.Buffer
		_ = png.Encode(&encoded, preview) // a bytes.Buffer cannot fail to write
		digest := fmt.Sprintf("%x", sha256.Sum256(raw))
		view.References.Images[index] = Reference{Path: ref.Path, SHA256: digest, MediaType: mediaType}
		view.Previews = append(view.Previews, Preview{Path: ref.Path, MIMEType: mediaType, SHA256: digest,
			Bytes: len(raw), Width: width, Height: height, DataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())})
	}
	return view, nil
}
