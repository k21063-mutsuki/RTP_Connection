package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"log"
	"time"

	"test/library"

	"gocv.io/x/gocv"
)

const (
	mtu              = 1460 // ここで分割する最大サイズを指定
	payloadType      = 0    // 映像用など(任意)
	ssrc             = 12345
	srcID            = 111
	dstID            = 222
	cameraDeviceID   = 0
	frameRate        = 60
	startSequenceNum = 1
)

func serverCallback(firstMeta library.RTPMetaData, registryFrames [][]byte, clientFrames [][]byte) [][]byte {
	log.Printf("FirstMeta stored:\n"+
		"  Version:        %d\n"+
		"  PayloadType:    %d\n"+
		"  SequenceNumber: %d\n"+
		"  Timestamp:      %d\n"+
		"  SSRC:           %d\n"+
		"  CSRC:           %v\n"+
		"  SrcID:          %d\n"+
		"  DstID:          %d\n"+
		"  PayloadLength:  %d\n"+
		"  ExtTimeStamp:  %d",
		firstMeta.Version,
		firstMeta.PayloadType,
		firstMeta.SequenceNumber,
		firstMeta.Timestamp,
		firstMeta.SSRC,
		firstMeta.CSRC,
		firstMeta.SrcID,
		firstMeta.DstID,
		firstMeta.PayloadLength,
		firstMeta.ExtTimestamp,
	)
	return registryFrames
}

func main() {
	// (A) サーバをgoroutineで起動

	rawCh := make(chan []byte, 1000)
	go library.UDPServer(serverCallback, rawCh)
	fmt.Println("UDP server started on :9000")

	// (B) Webカメラをオープン
	webcam, err := gocv.OpenVideoCapture(cameraDeviceID)
	if err != nil {
		log.Fatalf("Error opening video capture device: %v", err)
	}
	defer webcam.Close()

	// 表示用ウィンドウを作成
	window := gocv.NewWindow("Camera")
	defer window.Close()

	// 画像用Mat
	img := gocv.NewMat()
	defer img.Close()

	// sequenceNumber := startSequenceNum

	// 連続でフレームを取得して処理するループ
	for {
		if ok := webcam.Read(&img); !ok {
			log.Println("Webcam closed or no frame available")
			break
		}
		if img.Empty() {
			log.Println("Image Data Empty")
			continue
		}

		// JPEGエンコード処理
		buf := new(bytes.Buffer)
		imgData, err := img.ToImage()
		if err != nil {
			log.Println("failed to convert Mat to image.Image:", err)
			continue
		}
		if err := jpeg.Encode(buf, imgData, nil); err != nil {
			log.Println("Failed to encode JPEG:", err)
			continue
		}
		frameBytes := buf.Bytes()

		log.Printf("Encoded frame data length: %d", len(frameBytes))
		if len(frameBytes) >= 20 {
			log.Printf("Encoded frame first 20 bytes: %x", frameBytes[:20])
		}

		select {
		case rawCh <- frameBytes:
		default:
			// チャネルが満杯ならドロップ
		}

		var completePayload []byte

		completePayload = library.GetLatestData()

		// JPEGバイトデータを使ってIMDecodeで再びMatに変換
		decodedMat, err := gocv.IMDecode(completePayload, gocv.IMReadColor)
		if err != nil {
			log.Println("Failed to decode JPEG:", err)
			continue
		}
		if decodedMat.Empty() {
			log.Println("Decoded mat is empty")
			decodedMat.Close()
			continue
		}

		// ウィンドウに表示
		window.IMShow(decodedMat)
		// WaitKeyを呼ぶことでウィンドウのイベントループに時間を与える
		if window.WaitKey(1) >= 0 {
			break
		}
		decodedMat.Close()

		// フレームレート制御
		time.Sleep(time.Second / frameRate)
	}
}

// (H) クライアントのレスポンス受信コールバック
// func clientBatchCallback(results []library.TargetResponse) ([][]byte, error) {
// 	for _, tr := range results {
// 		fmt.Printf("[clientBatchCallback] to %s:%s => #Responses=%d\n",
// 			tr.Target.SourceIP, tr.Target.TargetIP, len(tr.Responses))
// 		for i, resp := range tr.Responses {
// 			fmt.Printf("  resp[%d] from %s => %d bytes: %q\n",
// 				i, resp.Addr, len(resp.Data), string(resp.Data))
// 		}
// 	}
// 	return nil, nil
// }
