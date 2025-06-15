package library

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

// サーバごとの結果をまとめる構造体
type TargetResponse struct {
	Addr     string // "IP:port"
	MetaData RTPMetaData
	Frames   [][]byte // Marker ビット単位で組み立てたフレーム
}

// cb は最終的に TargetResponse のスライスを受け取り、出力フレームを返すコールバック
type BatchCallbackFunc func([]TargetResponse) ([][]byte, error)

const (
	mtu              = 1460 // ここで分割する最大サイズを指定
	payloadType      = 0    // 映像用など(任意)
	ssrc             = 12345
	srcID            = 111
	dstID            = 222
	cameraDeviceID   = 0
	frameRate        = 60
	startSequenceNum = 1
	seq              = 1
)

// UDPClientBatch は単一のバイナリ data を受け取り、targets の各サーバへ送り、
// 受信した RTP パケットをサーバ別にフレーム組み立てしてコールバックを呼びます。
// func UDPClientBatch(
// 	data []byte,
// 	PayloadType int,
// 	timestampincrement int,
// 	cb BatchCallbackFunc,
// ) ([][]byte, error) {
// 	if len(os.Args) < 2 {
// 		fmt.Fprintf(os.Stderr, "Usage: %s <arg1> [<arg2> [<arg3>]]\n", os.Args[0])
// 		os.Exit(1)
// 	}

// 	// parseEntries で os.Args[2] から group2 を作成
// 	var group1 []NodeInfo
// 	var group2 []NodeInfo

// 	group1 = parseEntries(os.Args[1])
// 	if len(os.Args) > 2 {
// 		group2 = parseEntries(os.Args[2])
// 	}

// 	for i, ni := range group1 {
// 		log.Printf("Group1:  [%d] IP=%s, Port=%s, NodeID=%d\n", i, ni.IP, ni.Port, ni.NodeID)
// 	}

// 	for i, ni := range group2 {
// 		log.Printf("Group2:  [%d] IP=%s, Port=%s, NodeID=%d\n", i, ni.IP, ni.Port, ni.NodeID)
// 	}

// 	// 1) ローカル UDP ソケットを任意のポートで開く
// 	laddr, err := net.ResolveUDPAddr("udp", group1[0].IP+":"+"9001")
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to resolve local addr: %w", err)
// 	}
// 	conn, err := net.ListenUDP("udp", laddr)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to listen on UDP: %w", err)
// 	}
// 	defer conn.Close()

// 	// 2) group2 の各サーバごとに、データを MTU 単位で分割して RTP パケット化し送信
// 	const mtu = 1450
// 	packetsMap := make(map[string][][]byte, len(group2))

// 	for _, ni := range group2 {
// 		addrStr := net.JoinHostPort(ni.IP, ni.Port)
// 		// シーケンス番号をサーバごとにリセット
// 		seqNum := startSequenceNum
// 		// 1フレームのタイムスタンプ（例として固定 or カスタムロジック）
// 		timestamp := uint32(time.Now().UnixNano() / 1e6) // ms 単位の例

// 		// 分割数を計算
// 		numChunks := (len(data) + mtu - 1) / mtu

// 		var raws [][]byte
// 		for i := 0; i < numChunks; i++ {
// 			start := i * mtu
// 			end := start + mtu
// 			if end > len(data) {
// 				end = len(data)
// 			}
// 			chunk := data[start:end]

// 			// 最後のチャンクのみ Marker=true
// 			marker := (i == numChunks-1)

// 			// CreateRTPPacket の srcID に ni.NodeID をセット
// 			_, raw, _, err := CreateRTPPacket(
// 				uint8(PayloadType),
// 				uint16(seqNum),
// 				timestamp,
// 				ssrc,
// 				nil,               // CSRC リストがなければ nil
// 				uint16(ni.NodeID), // ← ここで NodeID を srcID に
// 				uint16(group1[0].NodeID),
// 				chunk,
// 				marker,
// 			)
// 			if err != nil {
// 				return nil, fmt.Errorf("failed to packetize chunk %d : %w", i, err)
// 			}
// 			raws = append(raws, raw)

// 			seqNum++
// 			timestamp += uint32(timestampincrement)
// 		}
// 		packetsMap[addrStr] = raws
// 		log.Printf("Generated %d RTP packets for server %s", len(raws), addrStr)
// 	}

// 	for addrStr, raws := range packetsMap {
// 		raddr, err := net.ResolveUDPAddr("udp", addrStr)
// 		txConn, _ := net.DialUDP("udp", nil, raddr)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to resolve addr %s: %w", addrStr, err)
// 		}

// 		sendRefusedCount := 0
// 		log.Printf("Sending %d packets to %s", len(raws), addrStr)
// 		for i, raw := range raws {
// 			_, err := conn.Write(raw)
// 			if err != nil {
// 				// connection refused の場合はカウント
// 				if opErr, ok := err.(*net.OpError); ok {
// 					if opErr.Err.Error() == "connection refused" || opErr.Err.Error() == "connect: connection refused" {
// 						sendRefusedCount++
// 						log.Printf("send connection refused encountered (%d times) to %s", sendRefusedCount, addrStr)
// 						if sendRefusedCount >= 5 {
// 							return nil, fmt.Errorf("connection refused encountered %d times while sending to %s", sendRefusedCount, addrStr)
// 						}
// 						// 少し待機して再試行
// 						time.Sleep(time.Second)
// 						continue
// 					}
// 				}
// 				return nil, fmt.Errorf("failed to send packet #%d to %s: %w", i, addrStr, err)
// 			}
// 			sendRefusedCount = 0
// 			log.Printf("Sent packet #%d to server %s", i, addrStr)
// 			txConn.Close()
// 		}
// 	}

// 	// 3) サーバごとにフレームを組み立てるためのマップ
// 	slideWindows := make(map[string][][]byte) // addr -> 完成フレーム
// 	currentFrames := make(map[string][]byte)  // addr -> 組み立て中フレーム

// 	// EOF 検知用
// 	done := make(map[string]bool) // addr -> EOF 受信済み

// 	// 全部のサーバから EOF が来るか、タイムアウトするまでループ
// 	// timeout := time.Now().Add(5 * time.Second)
// 	// conn.SetReadDeadline(timeout)

// 	connRefusedCount := 0

// 	for {
// 		buf := make([]byte, 1500)
// 		n, raddr, err := conn.ReadFromUDP(buf)
// 		if err != nil {
// 			// connection refused エラーのチェック
// 			if opErr, ok := err.(*net.OpError); ok {
// 				if opErr.Err.Error() == "connection refused" ||
// 					(opErr.Err != nil && opErr.Err.Error() == "connect: connection refused") {
// 					connRefusedCount++
// 					log.Printf("connection refused encountered (%d times) from %s", connRefusedCount, raddr)
// 					if connRefusedCount >= 5 {
// 						return nil, fmt.Errorf("connection refused encountered %d times, aborting", connRefusedCount)
// 					}
// 					_ = conn.SetReadDeadline(time.Now().Add(time.Second))
// 					continue
// 				}
// 			}
// 			// タイムアウトの場合の処理
// 			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
// 				// タイムアウト発生時は再設定して受信継続
// 				_ = conn.SetReadDeadline(time.Now().Add(time.Second))
// 				continue
// 			}
// 			return nil, fmt.Errorf("failed to read from %s: %w", raddr, err)
// 		}

// 		connRefusedCount = 0

// 		addrKey := raddr.String()

// 		// 通常フレームの組み立て

// 		// ParseRTPPacket は既存関数を流用
// 		pkt, err := ParseRTPPacket(buf[:n])
// 		if err != nil {
// 			continue
// 		}

// 		if bytes.Equal(pkt.Payload, []byte("CSCF")) {
// 			fmt.Printf("[UDPClientBatch] CSCF received from %s. Exiting without callback.\n", raddr)
// 			return nil, nil
// 		}

// 		if bytes.Equal(pkt.Payload, []byte("CSSF")) {
// 			fmt.Printf("[UDPClientBatch] CSCF received from %s. Exiting without callback.\n", raddr)
// 			return nil, nil
// 		}

// 		// EOF 検知
// 		if bytes.Equal(pkt.Payload, []byte("EOF")) {
// 			done[addrKey] = true
// 			// 全サーバ EOF なら受信ループ終了
// 			if len(done) == len(group2) {
// 				fmt.Printf("[UDPClientBatch] EOF received from %s\n", raddr)
// 				break
// 			}
// 			continue
// 		}

// 		currentFrames[addrKey] = append(currentFrames[addrKey], pkt.Payload...)
// 		if pkt.Marker {
// 			slideWindows[addrKey] = append(slideWindows[addrKey], currentFrames[addrKey])

// 			if len(slideWindows[addrKey]) > maxFrames {
// 				slideWindows[addrKey] = slideWindows[addrKey][len(slideWindows[addrKey])-maxFrames:]
// 			}

// 			currentFrames[addrKey] = nil
// 		}

// 	}

// 	// 4) TargetResponse スライスを作成
// 	var results []TargetResponse
// 	for addr, frames := range slideWindows {
// 		results = append(results, TargetResponse{Addr: addr, Frames: frames})
// 	}

// 	// 5) コールバック実行
// 	return cb(results)
// }

func UDPClientBatch(
	data []byte,
	PayloadType int,
	timestampIncrement int,
	cb BatchCallbackFunc,
) ([][]byte, error) {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <arg1> [<arg2>]\n", os.Args[0])
		os.Exit(1)
	}

	// parseEntries で os.Args[1]〜[2] から group1, group2 を作成
	group1 := parseEntries(os.Args[1])
	var group2 []NodeInfo
	if len(os.Args) > 2 {
		group2 = parseEntries(os.Args[2])
	}

	// ローカル受送信用ソケットをバインド（任意ポートでもOKなら ":0"）
	laddr, err := net.ResolveUDPAddr("udp", group1[0].IP+":9005")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local addr: %w", err)
	}
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP: %w", err)
	}
	defer conn.Close()

	// MTU 分割＋RTP 化
	packetsMap := make(map[string][][]byte, len(group2))
	for _, ni := range group2 {
		var raws [][]byte
		addrStr := net.JoinHostPort(ni.IP, ni.Port)
		seqNum := startSequenceNum
		timestamp := uint32(time.Now().UnixNano() / 1e6)

		log.Printf("Create RTP Packet\n")
		_, confpacket, _, err := CreateRTPPacket(
			uint8(PayloadType),
			uint16(seqNum),
			timestamp,
			ssrc,
			nil,
			uint32(group1[0].NodeID),
			uint32(ni.NodeID),
			[]byte("Cnogiration"),
			false,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create configration packet : %w", err)
		}

		raws = append(raws, confpacket)
		numChunks := (len(data) + mtu - 1) / mtu
		log.Printf("Create RTP Packet2\n")
		for i := 0; i < numChunks; i++ {
			start := i * mtu
			end := start + mtu
			if end > len(data) {
				end = len(data)
			}
			chunk := data[start:end]
			marker := (i == numChunks-1)

			_, raw, _, err := CreateRTPPacket2(
				uint8(PayloadType),
				uint16(seqNum),
				timestamp,
				ssrc,
				nil,
				chunk,
				marker,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to packetize chunk %d: %w", i, err)
			}
			raws = append(raws, raw)
			seqNum++
			timestamp += uint32(timestampIncrement)
		}
		packetsMap[addrStr] = raws
		log.Printf("Generated %d RTP packets for server %s", len(raws), addrStr)
	}

	// 送信：同じ conn から WriteToUDP
	for addrStr, raws := range packetsMap {
		raddr, err := net.ResolveUDPAddr("udp", addrStr)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve remote addr %s: %w", addrStr, err)
		}
		log.Printf("Sending %d packets to %s", len(raws), addrStr)

		sendRefused := 0
		for i, raw := range raws {
			if _, err := conn.WriteToUDP(raw, raddr); err != nil {
				if op, ok := err.(*net.OpError); ok && (op.Err.Error() == "connection refused" || op.Err.Error() == "connect: connection refused") {
					sendRefused++
					log.Printf("connection refused (%d) to %s", sendRefused, addrStr)
					if sendRefused >= 5 {
						return nil, fmt.Errorf("connection refused %d times to %s", sendRefused, addrStr)
					}
					time.Sleep(time.Second)
					i-- // 再試行
					continue
				}
				return nil, fmt.Errorf("failed to send packet #%d to %s: %w", i, addrStr, err)
			}
			sendRefused = 0
			log.Printf("Sent packet #%d to %s", i, addrStr)
		}
	}

	// 受信＆フレーム組み立て
	slideWindows := make(map[string][][]byte)
	currentFrames := make(map[string][]byte)
	done := make(map[string]bool)
	// conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// メタ情報保存用
	metaMap := make(map[string]RTPMetaData)
	haveMeta := make(map[string]bool)

	for {
		// log.Printf("Start Reciving Packets\n")
		// log.Printf("First MetaData :%x", haveFirstMeta)
		buf := make([]byte, 1500)
		n, raddr, err := conn.ReadFromUDP(buf)

		addrKey := raddr.String()

		if !haveMeta[addrKey] {
			parsed, err := ParseRTPPacket(buf[:n])
			if err != nil {
				log.Printf("ParseRTPPacket error from %s: %v", raddr, err)
				continue
			}

			metaMap[addrKey] = RTPMetaData{
				Version:        parsed.Version,
				PayloadType:    parsed.PayloadType,
				SequenceNumber: parsed.SequenceNumber,
				Timestamp:      parsed.Timestamp,
				SSRC:           parsed.SSRC,
				CSRC:           parsed.CSRC,
				SrcID:          parsed.SrcID,
				DstID:          parsed.DstID,
				ExtTimestamp:   parsed.ExtTimestamp,
				PayloadLength:  parsed.PayloadLength,
			}
			haveMeta[addrKey] = true

			log.Printf("MetaData: %+v\n", metaMap[addrKey])
			if bytes.Equal(parsed.Payload, []byte("CSCF")) ||
				bytes.Equal(parsed.Payload, []byte("CSSF")) ||
				bytes.Equal(parsed.Payload, []byte("RFD")) {
				haveMeta[addrKey] = false
				log.Printf("First MetaData Reset\n")
				return nil, nil
			}
			continue
		}

		pkt, err := ParseRTPPacket2(buf[:n])
		if err != nil {
			log.Printf("ParseRTPPacket2 error from %s: %v", addrKey, err)
			continue
		}

		log.Printf("Received from %s: payload=%d bytes, seq=%d, timestamp=%d, marker=%v, first 20byte=%x", addrKey, len(pkt.Payload), pkt.SequenceNumber, pkt.Timestamp, pkt.Marker, pkt.Payload[:20])
		// 特殊ペイロード
		if bytes.Equal(pkt.Payload, []byte("CSCF")) ||
			bytes.Equal(pkt.Payload, []byte("CSSF")) ||
			bytes.Equal(pkt.Payload, []byte("RFD")) {
			return nil, nil
		}
		// EOF
		if bytes.Equal(pkt.Payload, []byte("EOF")) {
			done[addrKey] = true
			if len(done) == len(group2) {
				break
			}
			continue
		}

		// 通常フレームの組み立て
		currentFrames[addrKey] = append(currentFrames[addrKey], pkt.Payload...)
		if pkt.Marker {
			frameData := currentFrames[addrKey]
			log.Printf("Marker frame complete from %s: frame size=%d bytes", addrKey, len(frameData))
			slideWindows[addrKey] = append(slideWindows[addrKey], currentFrames[addrKey])
			if len(slideWindows[addrKey]) > 2 {
				slideWindows[addrKey] = slideWindows[addrKey][len(slideWindows[addrKey])-2:]
			}
			currentFrames[addrKey] = nil
		}
	}

	// コールバック用に結果整形
	var results []TargetResponse
	for addr, frames := range slideWindows {
		results = append(results, TargetResponse{Addr: addr, MetaData: metaMap[addr], Frames: frames})
	}
	return cb(results)
}
