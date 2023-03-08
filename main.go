package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/adrg/sysfont"
	"github.com/flopp/go-findfont"
	"github.com/fogleman/gg"
	"github.com/gin-gonic/gin"
	"github.com/ka2n/ptouchgo"
	_ "github.com/ka2n/ptouchgo/conn/usb"
	"github.com/mpvl/unique"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

func Router(r *gin.Engine) {
	r.GET("/", index)
	r.GET("/print", index)
}

type SafePrinter struct {
	lock      sync.Mutex
	ser       ptouchgo.Serial
	status    *ptouchgo.Status
	connected bool
}

var printer SafePrinter
var usableFonts []string

func openPrinter(ser *ptouchgo.Serial) error {
	args := flag.Args()

	var err error
	if !printer.connected || ser == nil {
		*ser, err = ptouchgo.Open(args[0], 0, true)

		if err != nil {
			println("Failed to open printer:", err.Error())
			return (err)
		}
	}
	printer.connected = false

	fmt.Println("reading status")
	ser.RequestStatus()
	printer.status, err = ser.ReadStatus()
	if err != nil {
		printer.ser.Close()
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(printer.status)

	printer.connected = true

	return nil
}

func createImage(text string, font_path string, fontsize int, vheight int, transparent bool) (*image.Image, error) {
	fmt.Printf("creating image h= %d font=%s\n", vheight, font_path)
	var err error
	fontdata := goregular.TTF

	if font_path != "" {
		fontdata, err = ioutil.ReadFile(font_path)
		if err != nil {
			return nil, fmt.Errorf("could not read font: %v", err)
		}
	}

	// load the font with the freetype library
	f, err := opentype.Parse(fontdata)
	if err != nil {
		return nil, fmt.Errorf("could not parse font: %v", err)
	}

	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(fontsize),
		DPI:     72, // 72 is default value, as such fontsize 1:1 rendered pixels
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, err
	}
	defer face.Close()

	dc := gg.NewContext(100, 100)
	dc.SetFontFace(face)

	w, h := dc.MeasureString(text)
	fmt.Printf("width: %f; height: %f;\n", w, h)

	dc = gg.NewContext(int(w+40), vheight)
	if transparent {
		dc.SetRGBA(0, 0, 0, 0)
	} else {
		dc.SetRGB(1, 1, 1)
	}
	dc.Clear()
	dc.SetRGB(0, 0, 0)
	dc.SetFontFace(face)

	measure := font.MeasureString(face, text)
	metrics := face.Metrics()
	v_pos := float64(dc.Height())/2 + (math.Abs(float64(metrics.CapHeight))/64)/2

	fmt.Printf("v_pos %f / advance %f / font metric: %#v\n", v_pos, float64(measure), metrics)
	// canvas_height/2 + (ascend / 2)
	dc.DrawStringAnchored(text, (w+40)/2, v_pos, 0.5, 0)
	img := dc.Image()
	return &img, nil
}

func printLabel(chain bool, img *image.Image, ser *ptouchgo.Serial) error {
	if printer.status == nil || !printer.connected {
		return fmt.Errorf("cannot print without printer")
	}

	if printer.status.TapeWidth == 0 {
		return fmt.Errorf("cannot print without tape detected")
	}
	ser.TapeWidthMM = uint(printer.status.TapeWidth)

	dc := gg.NewContext((*img).Bounds().Dx(), 128)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	dc.DrawImageAnchored(*img, 0, 128/2, 0, 0.5)

	data, bytesWidth, err := ptouchgo.LoadRawImage(dc.Image(), printer.status.TapeWidth)
	if err != nil {
		return err
	}
	rasterLines := len(data) / bytesWidth
	// Set property
	err = ser.SetPrintProperty(rasterLines)
	if err != nil {
		return err
	}
	packedData, err := ptouchgo.CompressImage(data, bytesWidth)
	if err != nil {
		return err
	}

	err = ser.SetRasterMode()
	if err != nil {
		return (err)
	}

	err = ser.SetFeedAmount(0)
	if err != nil {
		return (err)
	}

	err = ser.SetCompressionModeEnabled(true)
	if err != nil {
		return (err)
	}

	err = ser.SetPrintMode(true, false)
	if err != nil {
		return (err)
	}

	highDPI := true
	err = ser.SetExtendedMode(false, !chain, false, highDPI, false)
	if err != nil {
		return (err)
	}

	err = ser.SendImage(packedData)
	if err != nil {
		return err
	}

	err = ser.PrintAndEject()
	if err != nil {
		return err
	}

	return nil
}

func to_base64(img *image.Image) string {
	buf := new(bytes.Buffer)
	png.Encode(buf, *img)

	mimeType := "data:image/png;base64,"
	base := base64.StdEncoding.EncodeToString(buf.Bytes())

	return mimeType + base
}

func index(c *gin.Context) {
	var err error
	status := gin.H{}
	should_print := c.Request.URL.Path == "/print"

	label := c.Query("label")
	font := c.Query("font")
	_, no_fonts := c.GetQuery("no_fonts")

	count := c.DefaultQuery("count", "1")
	defaultFontSize := 32
	if printer.status != nil && printer.status.TapeWidth != 0 {
		// margin seems to scale with 128px max tape width
		if printer.status.TapeWidth == 9 {
			defaultFontSize = 32
		} else if printer.status.TapeWidth == 12 {
			defaultFontSize = 48
		} else {
			defaultFontSize = int(48 / 12 * printer.status.TapeWidth)
		}
	}

	fontsize := c.DefaultQuery("fontsize", strconv.Itoa(defaultFontSize))
	chain_print := c.Query("chain")

	fmt.Printf("label: %s; count: %s; should_print =%v path=%s\n", label, count, should_print, c.Request.URL.Path)

	size := 0
	if fontsize == "" {
		fontsize = strconv.Itoa(defaultFontSize)
		size = defaultFontSize
	} else {
		size, err = strconv.Atoi(fontsize)
		if err != nil {
			size = defaultFontSize
			fontsize = strconv.Itoa(size)
		}
	}
	if size > 240 {
		size = 240
		fontsize = strconv.Itoa(size)
	}

	// pretend 12mm tape
	vmargin_px := int(128 * 12 / 24)

	printer.lock.Lock()
	defer printer.lock.Unlock()

	err = openPrinter(&printer.ser)
	if err != nil {
		status["err"] = err
	}

	if printer.status != nil {
		if printer.status.Error1 != 0 {
			status["err"] = "Printer Tape error. Cannot print"
		}

		if printer.status.Error2 != 0 {
			status["err"] = "Printer error2 state: %d. Press power-button once to reset Software Error"
		}
		if printer.status.Model != 0 {
			status["connected"] = true
			if printer.status.TapeWidth != 0 {
				// margin seems to scale with 128px max tape width
				vmargin_px = int(128 * printer.status.TapeWidth / 24)
			} else {
				status["err"] = "No tape detected. Cannot print"
			}
			printer.connected = true
		} else if printer.connected {
			printer.ser.Close()
		}
	}

	status["label"] = label
	fontPath := ""

	finder := sysfont.NewFinder(nil)

	font = strings.TrimSpace(font)
	if font != "" {
		fontPath, err = findfont.Find(font)
		if err != nil {
			fmt.Printf("Falling back to fontmatch")
			foundFont := finder.Match(font)
			fontPath = foundFont.Filename
		}
		fmt.Printf("Found '%s' in '%s'\n", font, fontPath)
		font = path.Base(fontPath)
	}

	if !no_fonts {
		status["fonts"] = usableFonts
	}
	status["font"] = font

	img, err := createImage(label, fontPath, size, vmargin_px, false)
	if err != nil {
		status["err"] = err
	}

	if count == "" {
		count = "1"
	}

	copies, err := strconv.Atoi(count)
	if err != nil {
		copies = 1
	}

	if should_print {
		for i := 1; i <= copies; i++ {
			err = printLabel(i != copies || chain_print == "checked", img, &printer.ser)
			if err != nil {
				status["err"] = err
				break
			}
		}
	}
	if should_print {
		url := "/?"
		paramPairs := c.Request.URL.Query()
		for key, values := range paramPairs {
			url += key + "=" + values[0] + "&"
		}
		c.Redirect(http.StatusFound, url)
		return
	}

	status["count"] = count
	status["fontsize"] = fontsize

	if chain_print == "checked" {
		status["chain"] = should_print
	}

	if img != nil {
		// see issue https://github.com/golang/go/issues/20536 on why using URL type
		status["image"] = template.URL(to_base64(img))
	}

	if printer.status != nil {
		status["status"] = printer.status
	}

	c.HTML(
		http.StatusOK,
		"index",
		status,
	)
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: ptouch-web [device]\n")
	fmt.Fprintf(os.Stderr, "device can be \"usb\" or \"/dev/rfcomm0\" or similiar\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Println("connection is missing.")
		os.Exit(1)
	}

	finder := sysfont.NewFinder(nil)
	for _, systemFont := range finder.List() {

		ext := path.Ext(systemFont.Filename)
		if systemFont.Name != "" && (ext == ".ttf" || ext == ".otf") {
			usableFonts = append(usableFonts, systemFont.Name)

			imagePath := path.Join("static/img/fonts/", systemFont.Name+".png")
			if fileExists(imagePath) {
				continue
			}

			img, err := createImage(systemFont.Name, systemFont.Filename, 20, 24, true)
			if err != nil {
				panic(err)
			}
			dc := gg.NewContextForImage(*img)
			err = dc.SavePNG(imagePath)
			if err != nil {
				panic(err)
			}
		}
	}
	sort.Strings(usableFonts)
	unique.Strings(&usableFonts)

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	r.Static("/css", "./static/css")
	r.Static("/js", "./static/js")
	r.Static("/img", "./static/img")

	r.LoadHTMLGlob("templates/*")
	Router(r)

	log.Println("Server started")
	r.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
