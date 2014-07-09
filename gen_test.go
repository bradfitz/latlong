// +build latlong_gen

package latlong

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"flag"
	"fmt"
	"go/format"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"testing"
	"time"

	"code.google.com/p/freetype-go/freetype/raster"
	"github.com/jonas-p/go-shp"
)

const file = "worldtz.png"

var (
	flagGenerate   = flag.Bool("generate", false, "Do generation")
	flagWriteImage = flag.Bool("write_image", false, "Write out a debug image")
	flagScale      = flag.Float64("scale", 32, "Scaling factor. This many pixels wide & tall per degree (e.g. scale 1 is 360 x 180). Increasingly this code assumes a scale of 32, though.")
)

func saveToPNGFile(filePath string, m image.Image) {
	log.Printf("Encoding image %s ...", filePath)
	f, err := os.Create(filePath)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer f.Close()
	b := bufio.NewWriter(f)
	err = png.Encode(b, m)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	err = b.Flush()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s OK.\n", filePath)
}

func cloneImage(i *image.RGBA) *image.RGBA {
	i2 := new(image.RGBA)
	*i2 = *i
	i2.Pix = make([]uint8, len(i.Pix))
	copy(i2.Pix, i.Pix)
	return i2
}

func loadImage(filename string) *image.NRGBA {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	im, _, err := image.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	return im.(*image.NRGBA)
}

var tab = crc32.MakeTable(crc32.IEEE + 1)

var width, height int

const alphaErased = 22 // magic alpha value to mean tile's been erased

// the returned zoneOfColor always has A == 256.
func worldImage(t *testing.T) (im *image.RGBA, zoneOfColor map[color.RGBA]string) {
	scale := *flagScale
	width = int(scale * 360)
	height = int(scale * 180)

	im = image.NewRGBA(image.Rect(0, 0, width, height))
	zoneOfColor = map[color.RGBA]string{}

	drawPoly := func(col color.RGBA, xys ...int) {
		painter := raster.NewRGBAPainter(im)
		painter.SetColor(col)
		r := raster.NewRasterizer(width, height)
		r.Start(raster.Point{X: raster.Fix32(xys[0]) << 8, Y: raster.Fix32(xys[1]) << 8})
		for i := 2; i < len(xys); i += 2 {
			r.Add1(raster.Point{X: raster.Fix32(xys[i]) << 8, Y: raster.Fix32(xys[i+1]) << 8})
		}
		r.Add1(raster.Point{X: raster.Fix32(xys[0]) << 8, Y: raster.Fix32(xys[1]) << 8})
		r.Rasterize(raster.NewMonochromePainter(painter))
	}

	sr, err := shp.Open("world/tz_world.shp")
	if err != nil {
		t.Fatalf("Error opening world/tz_world.shp: %v; unzip it from http://efele.net/maps/tz/world/tz_world.zip", err)
	}
	defer sr.Close()

	for sr.Next() {
		i, s := sr.Shape()
		p, ok := s.(*shp.Polygon)
		if !ok {
			t.Fatalf("Unknown shape %T", p)
		}
		zoneName := sr.ReadAttribute(i, 0)
		if zoneName == "uninhabited" {
			continue
		}
		if _, err := time.LoadLocation(zoneName); err != nil {
			t.Fatalf("Failed to load: %v (%v)", zoneName, err)
		}
		hash := crc32.Checksum([]byte(zoneName), tab)
		col := color.RGBA{uint8(hash >> 24), uint8(hash >> 16), uint8(hash >> 8), 255}
		if name, ok := zoneOfColor[col]; ok {
			if name != zoneName {
				log.Fatalf("Color %+v dup: %s and %s", col, name, zoneName)
			}
		} else {
			zoneOfColor[col] = zoneName
		}

		var xys []int
		for _, pt := range p.Points {
			xys = append(xys, int((pt.X+180)*scale), int((90-pt.Y)*scale))
		}
		drawPoly(col, xys...)
	}

	// adjust point from scale 32 to whatever the user is using.
	ap := func(x int) int { return x * int(scale) / 32 }
	// Fix some rendering glitches:
	// {186 205 234 255} = Europe/Rome
	drawPoly(color.RGBA{186, 205, 234, 255},
		ap(6156), ap(1468),
		ap(6293), ap(1596),
		ap(6293), ap(1598),
		ap(6156), ap(1540))
	// {136 136 180 255} = America/Boise
	drawPoly(color.RGBA{136, 136, 180, 255},
		ap(2145), ap(1468),
		ap(2189), ap(1468),
		ap(2189), ap(1536),
		ap(2145), ap(1536))
	// {120 247 14 255} = America/Denver
	drawPoly(color.RGBA{120, 247, 14, 255},
		ap(2167), ap(1536),
		ap(2171), ap(1536),
		ap(2217), ap(1714),
		ap(2204), ap(1724),
		ap(2160), ap(1537))
	return
}

func TestGenerate(t *testing.T) {
	if !*flagGenerate {
		t.Skip("skipping generationg without --generate flag")
	}

	im, zoneOfColor := worldImage(t)

	var gen bytes.Buffer
	gen.WriteString("// Auto-generated file. See README or Makefile.\n\npackage latlong\n\n")

	var zoneLookers bytes.Buffer
	zoneLookers.WriteString("var zoneLookers = []zoneLooker{\n")

	staticZoneIndex := map[string]uint16{}

	var zones []string
	for _, zone := range zoneOfColor {
		zones = append(zones, zone)
	}
	sort.Strings(zones)
	for i, zone := range zones {
		staticZoneIndex[zone] = uint16(i)
		fmt.Fprintf(&zoneLookers, "\tstaticZone(%q),\n", zone)
	}

	log.Printf("Num zones = %d", len(zones))

	var imo *image.RGBA // output image
	if *flagWriteImage {
		//saveToPNGFile(file, im)
		imo = cloneImage(im)
	}

	var solidTile = map[tileKey]string{} // -> zone name
	var solidTiles [6][]tileKey          // partitioned by sizeShift, then sorted
	var pixColorSeen = map[color.RGBA]bool{}

	for _, sizeShift := range []uint8{5, 4, 3, 2, 1, 0} {
		size := int(8 << sizeShift) // 256 ... 8
		xtiles := width / size
		ytiles := height / size
		sizeCount := map[int]int{} // num colors -> count
		skipSquares := 0
		for yt := 0; yt < ytiles; yt++ {
		XTile:
			for xt := 0; xt < xtiles; xt++ {
				var s map[color.RGBA]bool
				y0 := yt * size
				y1 := (yt + 1) * size
				x0 := xt * size
				x1 := (xt + 1) * size
				for y := y0; y < y1; y++ {
					for x := x0; x < x1; x++ {
						off := im.PixOffset(x, y)
						alpha := im.Pix[off+3]
						switch alpha {
						case 0:
							// transparent pixel
							continue
						case alphaErased:
							skipSquares++
							// This whole tile has been handled at a higher level. Skip.
							continue XTile
						case 255:
							// expected
						default:
							panic("Unexpected alpha value")
						}
						nc := color.RGBA{R: im.Pix[off], G: im.Pix[off+1], B: im.Pix[off+2], A: alpha}
						if s == nil {
							s = make(map[color.RGBA]bool)
						}
						s[nc] = true
					}
				}
				sizeCount[len(s)]++
				if len(s) == 1 {
					var c color.RGBA
					for c = range s {
						// get first (and only) key
					}
					if (c == color.RGBA{}) {
						panic("no color found for tile")
					}
					tk := newTileKey(sizeShift, uint16(xt), uint16(yt))
					zone := zoneOfColor[c]
					solidTile[tk] = zone
					solidTiles[sizeShift] = append(solidTiles[sizeShift], tk)
				}
				if sizeShift == 0 && len(s) > 1 {
					for c := range s {
						pixColorSeen[c] = true
					}
				}
				if len(s) == 1 || len(s) == 0 {
					for y := y0; y < y1; y += 8 {
						for x := x0; x < x1; x += 8 {
							off := im.PixOffset(x, y)
							im.Pix[off+3] = alphaErased
						}
					}
					if imo != nil {
						for y := y0; y < y1; y++ {
							for x := x0; x < x1; x++ {
								off := im.PixOffset(x, y)
								switch len(s) {
								case 1:
									// Yellow border
									if y == y0 || y == y1-1 || x == x0 || x == x1-1 {
										imo.Pix[off] = 255 - (128 - byte(size))
										imo.Pix[off+1] = 255 - (128 - byte(size))
										imo.Pix[off+2] = 0
										imo.Pix[off+3] = 255
									}
								case 0:
									// Fake ocean
									imo.Pix[off] = 0
									imo.Pix[off+1] = 0
									imo.Pix[off+2] = 128
									imo.Pix[off+3] = 255
								}

							}
						}
					}
				}
			}
		}
		log.Printf("For size %d, skipped %d, solids %d, dist: %+v", size, skipSquares, len(solidTile), sizeCount)
	}
	log.Printf("Pixel colors seen: %d", len(pixColorSeen))
	if *flagWriteImage {
		saveToPNGFile("regions.png", imo)
	}

	gen.WriteString("// worldTile maps from size to gzipLookers.\n")
	gen.WriteString("var worldTile = [6]*gzipLooker{\n")
	for size := 5; size >= 0; size-- {
		fmt.Fprintf(&gen, "\t%d: &gzipLooker{\n", size)
		var buf bytes.Buffer
		for _, tk := range solidTiles[size] {
			zoneName := solidTile[tk]
			if zoneName == "" {
				t.Fatalf("no zone name found for tile %d", tk)
			}
			idx, ok := staticZoneIndex[zoneName]
			if !ok {
				panic("zone should've been registered: " + zoneName)
			}
			binary.Write(&buf, binary.BigEndian, tk)
			binary.Write(&buf, binary.BigEndian, idx)
		}
		var zbuf bytes.Buffer
		zw := gzip.NewWriter(&zbuf)
		zw.Write(buf.Bytes())
		zw.Flush()

		log.Printf("size %d is %d entries: %d bytes (%d bytes compressed)", size, len(solidTiles[size]), buf.Len(), zbuf.Len())
		fmt.Fprintf(&gen, "\t\tgzipData: %q,\n", zbuf.Bytes())
		gen.WriteString("\t},\n")
	}
	gen.WriteString("}\n\n")

	zoneLookers.WriteString("}\n\n")

	// var zoneLookers = []zoneLooker{ ...
	gen.Write(zoneLookers.Bytes())

	fmt, err := format.Source(gen.Bytes())
	if err != nil {
		ioutil.WriteFile("z_gen_tables.go", gen.Bytes(), 0644)
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("z_gen_tables.go", fmt, 0644); err != nil {
		t.Fatal(err)
	}
}
