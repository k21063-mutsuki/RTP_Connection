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
	laddr, err := net.ResolveUDPAddr("udp", group1[0].IP+":9007")
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

		// log.Printf("Received from %s: payload=%d bytes, seq=%d, timestamp=%d, marker=%v, first 20byte=%x", addrKey, len(pkt.Payload), pkt.SequenceNumber, pkt.Timestamp, pkt.Marker, pkt.Payload[:20])
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
