package main

import (
	"image"
	"image/color"
	"log"
	"test/library"
	"time"

	"gocv.io/x/gocv"
)

func myCallback(result []library.TargetResponse) ([][]byte, error) {
	var outFrames [][]byte

	for _, tr := range result {
		log.Printf("Server: %s\nMetaData: %+v\n", tr.Addr, tr.MetaData)

		for _, frame := range tr.Frames {
			// frame は []byte そのもの (以前の resp.Data)
			// 1) 受信したバイト列を Mat にデコード
			imgMat, err := gocv.IMDecode(frame, gocv.IMReadColor)
			if err != nil {
				log.Printf("IMDecode error: %v", err)
				continue
			}
			if imgMat.Empty() {
				log.Printf("Frame is empty in CallBack")
				imgMat.Close()
				continue
			}

			log.Printf("Decode Success in CallBack")
			// Mat はループ内で必ず Close

			defer imgMat.Close()

			// 2) グレースケール変換
			gray := gocv.NewMat()
			defer gray.Close()
			gocv.CvtColor(imgMat, &gray, gocv.ColorBGRToGray)

			// 3) ブラー＆エッジ検出
			blurred := gocv.NewMat()
			defer blurred.Close()
			gocv.GaussianBlur(gray, &blurred, image.Pt(5, 5), 0, 0, gocv.BorderDefault)

			edges := gocv.NewMat()
			defer edges.Close()
			gocv.Canny(blurred, &edges, 20, 80)

			// 4) 輪郭抽出
			contours := gocv.FindContours(edges, gocv.RetrievalExternal, gocv.ChainApproxSimple)

			// 5) 元のカラー画像に枠描画
			for i := 0; i < contours.Size(); i++ {
				pts := contours.At(i)
				rect := gocv.BoundingRect(pts)
				if rect.Dx()*rect.Dy() < 100 {
					continue
				}
				gocv.Rectangle(&imgMat, rect, color.RGBA{R: 255, G: 0, B: 0, A: 0}, 2)
			}

			// 6) 再度 JPEG エンコード
			buf, err := gocv.IMEncode(".jpg", imgMat)
			if err != nil {
				log.Printf("IMEncode error: %v", err)
				continue
			}
			defer buf.Close()

			// 7) エンコードしたバイト列を返却リストへ
			outFrames = append(outFrames, buf.GetBytes())
		}
	}

	return outFrames, nil
}

func main() {
	window := gocv.NewWindow("Detection")
	// メインループ：一定間隔でリクエスト送信を繰り返す
	for {
		log.Printf("Start Client Lib\n")
		frames, err := library.UDPClientBatch([]byte("test"), 1, 300, myCallback)
		log.Printf("frames length: %d", len(frames))
		if err != nil {
			log.Printf("UDPClientBatch error: %v", err)
			break // エラー発生時にループを抜ける
		} else {
			// 受信した各フレームを GoCV で表示する
			for i, frameData := range frames {
				imgMat, err := gocv.IMDecode(frameData, gocv.IMReadColor)
				if err != nil {
					log.Printf("IMDecode error for frame #%d: %v", i, err)
					continue
				}
				if imgMat.Empty() {
					log.Printf("Frame #%d is empty", i)
					imgMat.Close()
					continue
				}

				log.Printf("Decode Success\n")
				// ウィンドウに表示
				window.IMShow(imgMat)
				// WaitKeyを呼ぶことでウィンドウのイベントループに時間を与える
				if window.WaitKey(1) >= 0 {
					break
				}
				imgMat.Close()
			}
		}

		// 次のリクエスト送信までの間隔（ここでは約33ms、30fps相当）
		time.Sleep(time.Second / 30)
	}
}
