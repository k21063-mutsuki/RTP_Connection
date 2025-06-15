package main

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	"log"
	"test/library"
	"time"

	"gocv.io/x/gocv"
)

// grayscaleImage は JPEG バイト列を受け取り、グレースケール化した JPEG バイト列を返します。
func grayscaleImage(input []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	draw.Draw(gray, bounds, img, bounds.Min, draw.Src)

	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, gray, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// serverCallback は UDPServer の受信コールバック。
// clientFrames に入ってきた各フレームをグレースケール化 → 傷検知 → 返却します。
func serverCallback(
	meta library.RTPMetaData,
	registry [][]byte,
	clientFrames [][]byte,
) [][]byte {
	var out [][]byte
	frames, _ := library.UDPClientBatch([]byte("test"), 1, 300, clientCallback)

	for _, frame := range frames {
		// (1) グレースケール化
		grayJPEG, err := grayscaleImage(frame)
		if err != nil {
			log.Printf("serverCallback: grayscale failed: %v", err)
			// 元のままパス
			out = append(out, frame)
			continue
		}
		out = append(out, grayJPEG)
	}
	return out
}

func clientCallback(
	responses []library.TargetResponse,
) ([][]byte, error) {
	var all [][]byte
	for _, resp := range responses {
		for _, f := range resp.Frames {
			all = append(all, f)
		}
	}
	return all, nil
}

func main() {

	rawCh := make(chan []byte, 1000)
	go library.UDPServer(serverCallback, rawCh)

	// for {
	// 	log.Printf("Server Moving\n")
	// 	time.Sleep(3 * time.Second)
	// }
	window := gocv.NewWindow("Detection")
	// メインループ：一定間隔でリクエスト送信を繰り返す
	// クライアント機能でフレーム送信（rawCh 経由）
	go func() {
		for {
			log.Printf("Start Client Lib\n")
			frames, err := library.UDPClientBatch([]byte("test"), 1, 300, clientCallback)
			log.Printf("frames length: %d", len(frames))
			if err != nil {
				log.Printf("UDPClientBatch error: %v", err)
				time.Sleep(time.Second)
				continue
			}
			// rawCh にフレームを送信
			for i, frame := range frames {
				select {
				case rawCh <- frame:
					log.Printf("Frame #%d sent to server via rawCh", i)
				default:
					log.Printf("rawCh full, dropped frame #%d", i)
				}
			}
			time.Sleep(time.Second / 30)
		}
	}()

	// メインループ：サーバ側から取得して表示
	for {
		frame := library.GetLatestData()
		if frame == nil {
			log.Printf("No frame yet")
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// JPEGデコード → Mat表示
		mat, err := gocv.IMDecode(frame, gocv.IMReadColor)
		if err != nil {
			log.Printf("IMDecode error: %v", err)
			continue
		}
		if mat.Empty() {
			log.Printf("Decoded frame is empty")
			mat.Close()
			continue
		}

		window.IMShow(mat)
		if window.WaitKey(1) >= 0 {
			mat.Close()
			break
		}
		mat.Close()
	}
}
