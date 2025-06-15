package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"log"
	"time"

	"test/library"

	"gocv.io/x/gocv"
)

// detectDefects はグレースケール JPEG に対して傷（エッジ）検知して赤線を描画する。
func detectDefects(input []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	grayImg, ok := img.(*image.Gray)
	if !ok {
		// 万一グレースケールでなければ変換
		bounds := img.Bounds()
		tmp := image.NewGray(bounds)
		draw.Draw(tmp, bounds, img, bounds.Min, draw.Src)
		grayImg = tmp
	}
	bounds := grayImg.Bounds()
	out := image.NewRGBA(bounds)
	draw.Draw(out, bounds, grayImg, bounds.Min, draw.Src)

	const threshold = 20
	for y := bounds.Min.Y; y < bounds.Max.Y-1; y++ {
		for x := bounds.Min.X; x < bounds.Max.X-1; x++ {
			c := grayImg.GrayAt(x, y).Y
			cx := grayImg.GrayAt(x+1, y).Y
			cy := grayImg.GrayAt(x, y+1).Y
			if diff(c, cx) > threshold || diff(c, cy) > threshold {
				out.Set(x, y, color.RGBA{255, 0, 0, 255})
			}
		}
	}
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, out, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func diff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// クライアント側コールバック：グレースケール画像に傷検知処理を施す
func clientCallback(
	responses []library.TargetResponse,
) ([][]byte, error) {
	var all [][]byte
	log.Printf("Moving Callback Function\n")
	for _, resp := range responses {
		for _, f := range resp.Frames {
			defected, err := detectDefects(f)
			if err != nil {
				log.Printf("clientCallback: defect detection failed: %v", err)
				all = append(all, f) // 失敗した場合はそのまま使う
			} else {
				all = append(all, defected)
			}
		}
	}
	return all, nil
}

func main() {
	window := gocv.NewWindow("Processed Frames")
	defer window.Close()

	for {
		frames, err := library.UDPClientBatch([]byte("test"), 1, 0, clientCallback)
		if err != nil {
			log.Printf("UDPClientBatch failed: %v", err)
			time.Sleep(time.Second / 30)
			continue
		}

		for i, frame := range frames {
			log.Printf("Frames Length %d\n", len(frame))
			// JPEG を image.Image にデコード
			// img, err := jpeg.Decode(bytes.NewReader(frame))
			mat, err := gocv.IMDecode(frame, gocv.IMReadColor)
			if err != nil {
				log.Printf("failed to decode frame #%d: %v", i, err)
				continue
			}
			if mat.Empty() {
				log.Printf("frame #%d is empty after IMDecode", i)
				continue
			}

			// image.Image → gocv.Mat に変換
			// mat, err := gocv.ImageToMatRGB(img)
			if err != nil {
				log.Printf("failed to convert frame #%d to Mat: %v", i, err)
				continue
			}
			// defer mat.Close()

			window.IMShow(mat)
			if window.WaitKey(1) >= 0 {
				mat.Close()
				break
			}
		}
	}
}
