package main

import(
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"fmt"
	"math"
	"os"
)

var(
    aspect = 0
    cropWidth = 0.0
    cropHeight = 0.0
    detailWeight = 0.2
    skinColor = [3]float64{0.78, 0.57, 0.44}
    skinBias = 0.01
    skinBrightnessMin = 0.2
    skinBrightnessMax = 1.0
    skinThreshold = 0.8
    skinWeight = 1.8
    saturationBrightnessMin = 0.25
    saturationBrightnessMax = 0.9
    saturationThreshold = 0.4
    saturationBias = 0.2
    saturationWeight = 0.3
    // step * minscale rounded down to the next power of two should be good
    scoreDownSample = 8
    step = 8
    scaleStep = 0.1
    minScale = 0.9
    maxScale = 1.0
    edgeRadius = 0.4
    edgeWeight = -20.0
    outsideImportance = -0.5
    ruleOfThirds = true
    prescale = true
    debug = false
)

type Score struct {
	Detail float64
	Saturation float64
	Skin float64
	Total float64
}

type Crop struct {
	X int
	Y int
	Width int
	Height int
	Score Score
}

type SubImager interface {
	SubImage(r image.Rectangle) image.Image
}

func thirds(x float64) float64 {
	x1 := int(x-(1.0/3.0) + 1.0)
	res := (float64(x1 % 2.0) * 0.5) - 0.5
	return res * 16.0
}

func importance(crop Crop, x, y int) float64 {
	if crop.X > x || x >= crop.X + crop.Width || crop.Y > y || y >= crop.Y + crop.Height {
		return outsideImportance
	}

	xf := float64(x - crop.X) / float64(crop.Width)
	yf := float64(y - crop.Y) / float64(crop.Height)

	px := math.Abs(0.5 - xf) * 2.0
	py := math.Abs(0.5 - yf) * 2.0

	dx := math.Max(px - 1.0 + edgeRadius, 0.0)
	dy := math.Max(py - 1.0 + edgeRadius, 0.0)
	d := (dx*dx + dy*dy) * edgeWeight

	s := 1.41 - math.Sqrt(px*px + py*py)
	if ruleOfThirds {
		s += (math.Max(0.0, s+d+0.5) * 1.2) * (thirds(px) + thirds(py))
	}

	return s + d
}

func score(output *image.Image, crop Crop) Score {
	invDownSample := 1.0 / float64(scoreDownSample)
	height := (*output).Bounds().Size().Y
	width := (*output).Bounds().Size().X
	score := Score{}

	for y := 0; y < height * scoreDownSample; y += scoreDownSample {
		for x := 0; x < width * scoreDownSample; x += scoreDownSample {
			p := int(math.Floor(float64(y) * invDownSample)) * width + int(math.Floor(float64(x) * invDownSample)) * 4
			imp := importance(crop, x, y)

			ny := int(math.Floor(float64(p / 4) / float64(width)))
			nx := 0
			if ny > 0 {
				nx = (p / 4) % ny
			} else {
				nx = p / 4
			}

			r, g, b, _ := (*output).At(nx, ny).RGBA()
			r8 := uint8(r>>8)
			g8 := uint8(g>>8)
			b8 := uint8(b>>8)

			score.Skin += float64(r8)/255.0 * (float64(g8)/255.0 + skinBias) * imp
			score.Detail += float64(g8)/255.0 * imp
			score.Saturation += float64(b8)/255.0 * (float64(g8)/255.0 + saturationBias) * imp
		}
	}

	score.Total = (score.Detail * detailWeight + score.Skin * skinWeight + score.Saturation * saturationWeight) / float64(crop.Width) / float64(crop.Height)
	return score
}

func writeImage(img *image.Image, name string) {
	fso, err := os.Create(name)
	if err != nil {
		panic(err)
	}
	defer fso.Close()

	jpeg.Encode(fso, (*img), &jpeg.Options{Quality: 90})
	fso.Close()
}

func cloneImage(src image.Image) (dst image.Image) {
	switch pic := src.(type) {
	case *image.Alpha:
		copy := *pic
		copy.Pix = make([]uint8, len(pic.Pix))
		dst = &copy
	case *image.Alpha16:
		copy := *pic
		copy.Pix = make([]uint8, len(pic.Pix))
		dst = &copy
	case *image.Gray:
		copy := *pic
		copy.Pix = make([]uint8, len(pic.Pix))
		dst = &copy
	case *image.Gray16:
		copy := *pic
		copy.Pix = make([]uint8, len(pic.Pix))
		dst = &copy
	case *image.NRGBA:
		copy := *pic
		copy.Pix = make([]uint8, len(pic.Pix))
		dst = &copy
	case *image.NRGBA64:
		copy := *pic
		copy.Pix = make([]uint8, len(pic.Pix))
		dst = &copy
	case *image.RGBA:
		copy := *pic
		copy.Pix = make([]uint8, len(pic.Pix))
		dst = &copy
	case *image.RGBA64:
		copy := *pic
		copy.Pix = make([]uint8, len(pic.Pix))
		dst = &copy
	default:
	}
	return
}

func analyse(img image.Image) Crop {
	o := cloneImage(img)

	edgeDetect(&img, &o)
	writeImage(&o, "/tmp/foo_step1.jpg")
	skinDetect(&img, &o)
	writeImage(&o, "/tmp/foo_step2.jpg")
	saturationDetect(&img, &o)
	writeImage(&o, "/tmp/foo_step3.jpg")

	var topCrop Crop
	topScore := -1.0
	cs := crops(&o)

	for _, crop := range cs {
		crop.Score = score(&o, crop)
		if crop.Score.Total > topScore {
			topCrop = crop
			topScore = crop.Score.Total
		}
	}

	fmt.Printf("Top crop: %v\n", topCrop)

	cropImage := img.(SubImager).SubImage(image.Rect(topCrop.X, topCrop.Y, topCrop.Width+topCrop.X, topCrop.Height+topCrop.Y))
	writeImage(&cropImage, "/tmp/foo_topcrop.jpg")

	return topCrop
}

func crop(img image.Image, width, height int) error {
	if width == 0 && height == 0 {
		return errors.New("Expect either a height or width")
	}

	scale := math.Min(float64(img.Bounds().Size().X) / float64(width), float64(img.Bounds().Size().Y) / float64(height))
	cropWidth, cropHeight = math.Floor(float64(width) * scale), math.Floor(float64(height) * scale)
	minScale = math.Min(maxScale, math.Max(1.0 / scale, minScale))

	fmt.Printf("original resolution: %dx%d\n", img.Bounds().Size().X, img.Bounds().Size().Y)
	fmt.Printf("scale: %f, cropw: %f, croph: %f, minscale: %f\n", scale, cropWidth, cropHeight, minScale)

	analyse(img)

	return nil
}

func saturation(c color.Color) float64 {
	r, g, b, _ := c.RGBA()

	maximum := math.Max(math.Max(float64(r)/255, float64(g)/255), float64(b)/255)
	minimum := math.Min(math.Min(float64(r)/255, float64(g)/255), float64(b)/255)

	if (maximum == minimum) {
		return 0
	}

	l := (maximum + minimum) / 2
	d := maximum - minimum

	if l > 0.5 {
		return d / (2-maximum-minimum)
	} else {
		return d / (maximum+minimum)
	}
}

func cie(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	r8 := uint8(r>>8)
	g8 := uint8(g>>8)
	b8 := uint8(b>>8)

	return 0.5126*float64(b8) + 0.7152*float64(g8) + 0.0722*float64(r8)
}

func skinCol(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	r8 := uint8(r>>8)
	g8 := uint8(g>>8)
	b8 := uint8(b>>8)

	mag := math.Sqrt(float64(r8*r8) + float64(g8*g8) + float64(b8*b8))
	rd := float64(r8)/mag - skinColor[0]
	gd := float64(g8)/mag - skinColor[1]
	bd := float64(b8)/mag - skinColor[2]

	d := math.Sqrt(float64(rd*rd) + float64(gd*gd) + float64(bd*bd))
	return 1.0 - d
}

func edgeDetect(i *image.Image, o *image.Image) {
	w := (*i).Bounds().Size().X
	h := (*i).Bounds().Size().Y

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var lightness float64

			if x == 0 || x >= w-1 || y == 0 || y >= h-1 {
				lightness = cie((*i).At(x, y))
			} else {
				lightness = cie((*i).At(x, y)) * 4.0 -
							cie((*i).At(x, y-1)) -
							cie((*i).At(x-1, y)) -
							cie((*i).At(x+1, y)) -
							cie((*i).At(x, y+1))
			}

			if lightness < 0 {
				continue
			}

			nc := color.RGBA{uint8(lightness), uint8(lightness), uint8(lightness), 255}
			(*o).(*image.RGBA).Set(x, y, nc)
		}
	}
}

func skinDetect(i *image.Image, o *image.Image) {
	w := (*i).Bounds().Size().X
	h := (*i).Bounds().Size().Y

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			lightness := cie((*i).At(x, y)) / 255.0
			skin := skinCol((*i).At(x, y))

			if skin > skinThreshold && lightness >= skinBrightnessMin && lightness <= skinBrightnessMax {
				r := (skin - skinThreshold) * (255.0/(1.0 - skinThreshold))
				_, g, b, a := (*i).At(x, y).RGBA()
				nc := color.RGBA{uint8(r), uint8(g>>8), uint8(b>>8), uint8(a>>8)}
				(*o).(*image.RGBA).Set(x, y, nc)
			} else {
				_, g, b, a := (*i).At(x, y).RGBA()
				nc := color.RGBA{0, uint8(g>>8), uint8(b>>8), uint8(a>>8)}
				(*o).(*image.RGBA).Set(x, y, nc)
			}
		}
	}
}

func saturationDetect(i *image.Image, o *image.Image) {
	w := (*i).Bounds().Size().X
	h := (*i).Bounds().Size().Y

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			lightness := cie((*i).At(x, y)) / 255.0
			saturation := saturation((*i).At(x, y))

			if saturation > saturationThreshold && lightness >= saturationBrightnessMin && lightness <= saturationBrightnessMax {
				b := (saturation - saturationThreshold) * (255.0/(1.0 - saturationThreshold))
				r, g, _, a := (*i).At(x, y).RGBA()
				nc := color.RGBA{uint8(r>>8), uint8(g>>8), uint8(b), uint8(a>>8)}
				(*o).(*image.RGBA).Set(x, y, nc)
			} else {
				r, g, _, a := (*i).At(x, y).RGBA()
				nc := color.RGBA{uint8(r>>8), uint8(g>>8), 0, uint8(a>>8)}
				(*o).(*image.RGBA).Set(x, y, nc)
			}
		}
	}
}

func crops(i *image.Image) []Crop {
	res := []Crop{}
	width := (*i).Bounds().Size().X
	height := (*i).Bounds().Size().Y

	//minDimension := math.Min(float64(width), float64(height))
	cropW := cropWidth //|| minDimension
	cropH := cropHeight //|| minDimension

	for scale := maxScale; scale >= minScale; scale -= scaleStep {
		for y := 0; float64(y)+cropH*scale <= float64(height); y += step {
			for x := 0; float64(x)+cropW*scale <= float64(width); x += step {
				res = append(res, Crop{
					X: x,
					Y: y,
					Width: int(cropW*scale),
					Height: int(cropH*scale),
				})
			}
		}
	}

	return res
}

func main() {
	fi, _ := os.Open("/tmp/foo.png")
	defer fi.Close()

	img, _, err := image.Decode(fi)
	if err != nil {
		panic(err)
		return
	}

	crop(img, 250, 250)
}
