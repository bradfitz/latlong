// +build latlong_gen

package latlong

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"go/format"

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

func TestGenerate(t *testing.T) {
	if !*flagGenerate {
		t.Skip("skipping generationg without --generate flag")
	}
	scale := *flagScale
	width = int(scale * 360)
	height = int(scale * 180)

	im := image.NewRGBA(image.Rect(0, 0, width, height))

	var gen bytes.Buffer
	gen.WriteString("// Auto-generated file. See README or Makefile.\n\npackage latlong\n\n")

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
		log.Fatalf("Error opening world/tz_world.shp: %v; unzip it from http://efele.net/maps/tz/world/tz_world.zip")
	}
	defer sr.Close()

	var zoneOfColor = map[color.RGBA]string{}

	var failZone int
	for sr.Next() {
		i, s := sr.Shape()
		p, ok := s.(*shp.Polygon)
		if !ok {
			log.Printf("Unknown shape %T", p)
			continue
		}
		zoneName := sr.ReadAttribute(i, 0)
		if zoneName == "uninhabited" {
			continue
		}
		if _, err := time.LoadLocation(zoneName); err != nil {
			t.Logf("Failed to load: %v (%v)", zoneName, err)
			failZone++
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

	for c, name := range zoneOfColor {
		log.Printf("%v = %s\n", c, name)
	}
	log.Printf("Num zones = %d (%d failed to load)", len(zoneOfColor), failZone)

	var imo *image.RGBA // output image
	if *flagWriteImage {
		saveToPNGFile(file, im)
		imo = cloneImage(im)
	}

	for _, sizeShift := range []uint8{5, 4, 3, 2, 1, 0} {
		size := int(8 << sizeShift) // 256 ... 8
		xtiles := width / size
		ytiles := height / size
		sizeCount := map[int]int{} // num colors -> count
		for yt := 0; yt < ytiles; yt++ {
			for xt := 0; xt < xtiles; xt++ {
				s := map[color.RGBA]bool{}
				y0 := yt * size
				y1 := (yt + 1) * size
				x0 := xt * size
				x1 := (xt + 1) * size
				for y := y0; y < y1; y++ {
					for x := x0; x < x1; x++ {
						off := im.PixOffset(x, y)
						if im.Pix[off+3] == 0 {
							// transparent pixel; ocean or already erased.
							continue
						}
						nc := color.RGBA{R: im.Pix[off], G: im.Pix[off+1], B: im.Pix[off+2]}
						if (nc != color.RGBA{}) {
							s[nc] = true
						}
					}
				}
				sizeCount[len(s)]++
				if imo != nil && (len(s) == 1 || len(s) == 0) {
					for y := y0; y < y1; y++ {
						for x := x0; x < x1; x++ {
							off := im.PixOffset(x, y)
							switch len(s) {
							case 1:
								// Yellow border
								copy(im.Pix[off:], "\x01\x00\x00\x00")
								if y == y0 || y == y1-1 || x == x0 || x == x1-1 {
									imo.Pix[off] = 255 - (128 - byte(size))
									imo.Pix[off+1] = 255 - (128 - byte(size))
									imo.Pix[off+2] = 0
									imo.Pix[off+3] = 255
								}
							case 0:
								if size == 256 {
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
		log.Printf("For size %d, dist: %+v", size, sizeCount)
	}
	if *flagWriteImage {
		saveToPNGFile("regions.png", imo)
	}
	fmt, err := format.Source(gen.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile("z_gen_tables.go", fmt, 0644); err != nil {
		t.Fatal(err)
	}
}
