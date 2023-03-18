package main

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"unsafe"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type Input struct {
	Text string
	Face font.Face
}

func findFontByGlyph(glyph rune, availableFaces []font.Face) font.Face {
	for i := len(availableFaces) - 1; i >= 0; i-- {
		//_, ok := availableFaces[i].GlyphAdvance(glyph)
		if open_face, ok := availableFaces[i].(*opentype.Face); ok {
			e := reflect.ValueOf(open_face)

			rf := e.Elem().FieldByName("f")
			font := reflect.NewAt(rf.Type(), unsafe.Pointer(rf.UnsafeAddr())).Interface().(**opentype.Font)

			var buf sfnt.Buffer

			// idx is 0 if glyph does not exist
			idx, err := (*font).GlyphIndex(&buf, glyph)
			if err == nil && idx != 0 {
				return availableFaces[i]
			}
		} else {
			panic("Unsupported font")
		}
	}
	return availableFaces[0]
}

func SplitByFontGlyphs(input string, availableFaces []font.Face) []Input {
	var splitTexts []Input
	currentText := Input{Text: input, Face: availableFaces[len(availableFaces)-1]}
	offset := 0

	for i, r := range []rune(input) {
		selectedFace := findFontByGlyph(r, availableFaces)

		if currentText.Face == selectedFace {
			continue
		}

		// new face is needed
		if i != 0 {
			currentText.Text = currentText.Text[0 : i-offset]
			splitTexts = append(splitTexts, currentText)
		}

		// create new currentText
		currentText = Input{Text: input[i:], Face: selectedFace}
		offset = i
	}

	splitTexts = append(splitTexts, currentText)
	return splitTexts
}

func MeasureStringFromSplitInput(input []Input) float64 {
	var advance fixed.Int26_6 = 0

	for _, in := range input {
		advance += font.MeasureString(in.Face, in.Text)
	}
	return float64(advance) / 64
}

func OpenFaceFromPath(fontPath string, fontsize int) (font.Face, error) {
	fontdata, err := ioutil.ReadFile(fontPath)
	if err != nil {
		return nil, fmt.Errorf("could not read font: %v", err)
	}
	return OpenFaceFromData(fontdata, fontsize)
}

func OpenFaceFromData(data []byte, fontsize int) (font.Face, error) {
	// load the font with the freetype library
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("could not parse font: %v", err)
	}

	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(fontsize),
		DPI:     72, // 72 is default value, as such fontsize 1:1 rendered pixels
		Hinting: font.HintingNone,
	})
}
