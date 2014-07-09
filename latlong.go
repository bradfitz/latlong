/*
Copyright 2014 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package latlong maps from a latitude and longitude to a timezone.
//
// It uses the data from http://efele.net/maps/tz/world/ compressed down
// to an internal form optimized for low memory overhead and fast lookups
// at the expense of perfect accuracy when close to borders. The data files
// are compiled in to this package and do not require explicit loading.
package latlong

// LookupZoneName returns the timezone name at the given latitude and
// longitude. The returned name is either the empty string (if not
// found) or a name suitable for passing to time.LoadLocation. For
// example, "America/New_York".
func LookupZoneName(lat, long float64) string {
	panic("TODO")
}

type zoneLooker interface {
	LookupZone(lat, long float64) (zone string, ok bool)
}

type staticZone string

func (z staticZone) LookupZone(lat, long float64) (zone string, ok bool) {
	return string(z), true
}

type worldGrid struct {
	tile map[tileKey]uint16 // value indexes into zone
	zone []zoneLooker
}

// A tilekey is a packed 32 bit integer where:
// 3 high bits: tile size: 8<<n (8 to 256 for n=0-5)
// bits 0-13 bits: x tile position
// bits 14-27 bits: y tile position
// bit 28: unused
// bit 31,30,29: tile size
// ssss
type tileKey uint32

// size is 0, 1, 2, or 3
func newTileKey(size uint8, x, y uint16) tileKey {
	return tileKey(size&7)<<28 |
		tileKey(y&(1<<14-1))<<14 |
		tileKey(x&(1<<14-1))
}

func (v tileKey) size() uint8 {
	return byte(v >> 28)
}

func (v tileKey) x() uint16 {
	return uint16(v & (1<<14 - 1))
}

func (v tileKey) y() uint16 {
	return uint16((v >> 14) & (1<<14 - 1))
}

type tileLooker struct {
	tile tileKey
	idx  uint16 // index into tileMapper
}

type gzipLooker struct {
	gzipData string       // compressed [tilekey][uint16_idx], repeated
	tiles    []tileLooker // lazily populated
}

// A twoBitTile represents a fully opaque 8x8 grid tile that only has
// two colors. The idx represents the indexes of the two colors (the palette)
// table, and the rows are the bits.
type twoBitTile struct {
	idx  [2]uint16 // bit 0 and bit 1's index into zoneLookers
	rows [8]uint8  // [y], then 1<<x.
}
