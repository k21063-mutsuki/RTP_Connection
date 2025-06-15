package library

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ----------------------------------
// データ構造や変数は同じ
// ----------------------------------

// type RTPPacketData struct {
// 	Version        uint8
// 	PayloadType    uint8
// 	SequenceNumber uint16
// 	Timestamp      uint32
// 	SSRC           uint32
// 	CSRC           []uint32
// 	Marker         bool
// 	Extension      bool
// 	ExtProfile     uint16
// 	SrcID          uint32
// 	DstID          uint32
// 	ExtTimestamp   uint64
// 	PayloadLength  uint32
// 	Payload        []byte
// }

type RTPMetaData struct {
	Version        uint8
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
	CSRC           []uint32
	SrcID          uint32
	DstID          uint32
	ExtTimestamp   uint64
	PayloadLength  uint32
}

var clientChannels = make(map[string]chan []byte)
var mu sync.Mutex

var (
	Registry [][]byte // PayloadType==0 の各フレーム
	// LastParsed RTPPacketData
	mu2 sync.Mutex // Registry, LastParsed 排他制御
)

type NodeInfo struct {
	IP     string
	Port   string
	NodeID int
}

const maxFrames = 2

type CallbackFunc func(meta RTPMetaData, registryFrames [][]byte, clientFrames [][]byte) [][]byte

const maxClientFrames = 2

// この関数で受信処理を行い、PayloadType==0 のフレームは Registry にためるだけ
func HandleConnection(addr string, conn *net.UDPConn, ch chan []byte, cb CallbackFunc) {
	var aggPayload0 []byte // PayloadType==0のフレーム用連結バッファ

	var aggPayloadNon0 []byte
	var lastParsedNon0 RTPPacketData2

	// PayloadType!=0 用のフレーム保持
	var clientFrames [][]byte
	var cfMu sync.Mutex
	var lastClientFrameTime time.Time

	// clientFrames クリア用ゴルーチン（省略可）
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		clearThreshold := 1 * time.Second

		for range ticker.C {
			cfMu.Lock()
			if !lastClientFrameTime.IsZero() && time.Since(lastClientFrameTime) > clearThreshold {
				clientFrames = nil
			}
			cfMu.Unlock()
		}
	}()

	var firstMeta RTPMetaData
	haveFirstMeta := false

	const minClientFrames = 2
	const maxClientFrames = 2

	// メイン受信ループ
	for packet := range ch {
		if !haveFirstMeta {
			log.Printf("Recived Packet from %s\n", addr)
			log.Printf("Parsed RTP Packet\n")
			parsed, err := ParseRTPPacket(packet)
			if err != nil {
				log.Printf("ParseRTPPacket error from %s: %v", addr, err)
				continue
			}
			firstMeta.Version = parsed.Version
			firstMeta.PayloadType = parsed.PayloadType
			firstMeta.SequenceNumber = parsed.SequenceNumber
			firstMeta.Timestamp = parsed.Timestamp
			firstMeta.SSRC = parsed.SSRC
			firstMeta.CSRC = parsed.CSRC
			firstMeta.SrcID = parsed.SrcID
			firstMeta.DstID = parsed.DstID
			firstMeta.PayloadType = parsed.PayloadType
			firstMeta.PayloadLength = parsed.PayloadLength
			haveFirstMeta = true
			aggPayload0 = nil
			aggPayloadNon0 = nil

			log.Printf("First MetaData: %x\n", firstMeta)

			continue
		}
		log.Printf("Parsed RTP Packet2\n")
		parsed, err := ParseRTPPacket2(packet)
		if err != nil {
			log.Printf("ParseRTPPacket2 error from %s: %v", addr, err)
			continue
		}

		// log.Printf("Received RTP packet from %s - Seq: %d, PayloadType: %d, Marker: %v", addr, parsed.SequenceNumber, parsed.PayloadType, parsed.Marker)

		if parsed.PayloadType == 0 {
			// PayloadType==0 → 1フレーム分のデータを連結
			aggPayload0 = append(aggPayload0, parsed.Payload...)
			// mu2.Lock()
			// LastParsed = parsed
			// mu2.Unlock()

			if parsed.Marker {
				// 1フレーム完成
				mu2.Lock()
				Registry = append(Registry, aggPayload0)
				log.Printf("Register Frame Data For Server: %d...", len(aggPayload0))

				if len(Registry) > maxFrames {
					Registry = Registry[len(Registry)-maxFrames:]
				}
				mu2.Unlock()

				// 応答送信（省略可）
				udpAddr, _ := net.ResolveUDPAddr("udp", addr)

				// ← ここから RFD(Register Frame Data) パケット生成＆送信を追加
				eofPayload := []byte("RFD")
				// シーケンス番号とタイムスタンプは適宜調整
				seq := lastParsedNon0.SequenceNumber + 1
				ts := lastParsedNon0.Timestamp + 1
				_, eofPkt, _, err := CreateRTPPacket(
					lastParsedNon0.PayloadType,
					seq,
					ts,
					lastParsedNon0.SSRC,
					lastParsedNon0.CSRC,
					uint32(firstMeta.DstID),
					uint32(firstMeta.SrcID),
					eofPayload,
					true, // EOFパケットにもマーカーを立てる
				)
				if err != nil {
					log.Printf("failed to create EOF RTP packet: %v", err)
				}
				conn.WriteToUDP(eofPkt, udpAddr)

				// バッファ初期化
				aggPayload0 = nil
			}
		} else if parsed.PayloadType == 1 {
			log.Printf("PayloadType = 1\n")
			// PayloadType==1 の場合の処理
			aggPayloadNon0 = append(aggPayloadNon0, parsed.Payload...)
			lastParsedNon0 = parsed

			if parsed.Marker {
				udpAddr, _ := net.ResolveUDPAddr("udp", addr)
				// 1フレーム完成 → clientFrames に追加
				cfMu.Lock()
				clientFrames = append(clientFrames, aggPayloadNon0)
				lastClientFrameTime = time.Now()
				// 最大保持数を超えたら、古いものを落とす
				if len(clientFrames) > maxClientFrames {
					clientFrames = clientFrames[1:]
				}
				framesCount := len(clientFrames)
				cfMu.Unlock()

				if framesCount < minClientFrames {
					// 次のフレーム受信に向けてリセットだけ
					log.Printf("clientFrames Length: %d\n", framesCount)
					aggPayloadNon0 = nil
					seq := lastParsedNon0.SequenceNumber + 1
					ts := lastParsedNon0.Timestamp + 1
					csfPayload := []byte("CSCF")
					_, csfPkt, _, err := CreateRTPPacket(
						lastParsedNon0.PayloadType,
						seq,
						ts,
						lastParsedNon0.SSRC,
						lastParsedNon0.CSRC,
						uint32(firstMeta.DstID),
						uint32(firstMeta.SrcID),
						csfPayload,
						true, // EOFパケットにもマーカーを立てる
					)
					if err != nil {
						log.Printf("failed to create EOF RTP packet: %v", err)
					}
					conn.WriteToUDP(csfPkt, udpAddr)
					haveFirstMeta = false
					log.Printf("FirstMeta Reset\n")
					continue
				}

				// ★ ここで、Registry のフレームが3つ以上貯まっているかチェックする
				mu2.Lock()
				regCount := len(Registry)
				mu2.Unlock()
				if regCount < 2 {
					// 次のフレーム受信に向けてリセットだけ
					log.Printf("ServerFrames Length: %d\n", regCount)
					aggPayloadNon0 = nil
					seq := lastParsedNon0.SequenceNumber + 1
					ts := lastParsedNon0.Timestamp + 1
					csfPayload := []byte("CSSF")
					_, csfPkt, _, err := CreateRTPPacket(
						lastParsedNon0.PayloadType,
						seq,
						ts,
						lastParsedNon0.SSRC,
						lastParsedNon0.CSRC,
						uint32(firstMeta.DstID),
						uint32(firstMeta.SrcID),
						csfPayload,
						true, // EOFパケットにもマーカーを立てる
					)
					if err != nil {
						log.Printf("failed to create EOF RTP packet: %v", err)
					}
					conn.WriteToUDP(csfPkt, udpAddr)
					haveFirstMeta = false
					log.Printf("FirstMeta Reset\n")
					continue
				}

				firstMeta.ExtTimestamp = uint64(parsed.ExtTimestamp)

				// Registry に十分なフレームが揃った場合、clientFrames をコピーしてコールバック実行
				cfMu.Lock()
				framesCopy := make([][]byte, len(clientFrames))
				copy(framesCopy, clientFrames)
				cfMu.Unlock()

				respData := cb(firstMeta, Registry, framesCopy)

				var results [][]byte
				seq := uint16(0)
				ts := uint32(0)
				_, configPkt, _, err := CreateRTPPacket(
					lastParsedNon0.PayloadType,
					seq,
					ts,
					lastParsedNon0.SSRC,
					lastParsedNon0.CSRC,
					uint32(firstMeta.DstID),
					uint32(firstMeta.SrcID),
					[]byte("Config"),
					true, // EOFパケットにもマーカーを立てる
				)
				if err != nil {
					log.Printf("failed to create Config RTP packet: %v", err)
				}

				results = append(results, configPkt)
				for idx, frameBytes := range respData {
					// --- (1) フレームは既に JPEG バイト列と仮定 ---
					seq++
					ts++
					encodedData := frameBytes

					// --- (2) MTU 単位で分割 ---
					mtu := 1450
					numChunks := len(encodedData) / mtu
					if len(encodedData)%mtu != 0 {
						numChunks++
					}

					// --- (3) 各チャンクを RTP パケット化 ---
					for i := 0; i < numChunks; i++ {
						start := i * mtu
						end := start + mtu
						if end > len(encodedData) {
							end = len(encodedData)
						}
						chunk := encodedData[start:end]

						// 最後のチャンクのみ Marker=true
						marker := (i == numChunks-1)

						// シーケンス番号／タイムスタンプはフレーム＋チャンク単位で調整
						// seq := req.SequenceNumber + uint16(idx*100+i)
						// ts := req.Timestamp + uint32(idx+1)

						_, rtpPkt, _, err := CreateRTPPacket2(
							uint8(firstMeta.PayloadType),
							seq,
							ts,
							lastParsedNon0.SSRC,
							lastParsedNon0.CSRC,
							chunk,
							marker,
						)
						if err != nil {
							log.Printf("Frame #%d chunk #%d: RTP packetization failed: %v", idx, i, err)
							continue
						}
						results = append(results, rtpPkt)
					}
				}

				// EOF パケット生成＆送信
				eofPayload := []byte("EOF")
				// seq := lastParsedNon0.SequenceNumber + 1
				// ts := lastParsedNon0.Timestamp + 1
				seq++
				ts++
				_, eofPkt, _, err := CreateRTPPacket2(
					lastParsedNon0.PayloadType,
					seq,
					ts,
					lastParsedNon0.SSRC,
					lastParsedNon0.CSRC,
					eofPayload,
					true,
				)
				if err != nil {
					log.Printf("failed to create EOF RTP packet: %v", err)
				}
				results = append(results, eofPkt)

				log.Printf("---\n")
				log.Printf("CallBack Function Fin\n")
				log.Printf("CallBack Function Response Length: %d\n", len(respData))
				log.Printf("---\n")

				// mu2.Lock()
				// Registry = nil
				// mu2.Unlock()

				// 応答送信
				for i, pkt := range results {
					if _, err := conn.WriteToUDP(pkt, udpAddr); err != nil {
						log.Printf("failed to send response packet #%d to %s: %v", i, addr, err)
					}
				}

				// // EOF パケット生成＆送信
				// eofPayload := []byte("EOF")
				// seq := lastParsedNon0.SequenceNumber + 1
				// ts := lastParsedNon0.Timestamp + 1
				// _, eofPkt, _, err := CreateRTPPacket(
				// 	lastParsedNon0.PayloadType,
				// 	seq,
				// 	ts,
				// 	lastParsedNon0.SSRC,
				// 	lastParsedNon0.CSRC,
				// 	uint32(firstMeta.DstID),
				// 	uint32(firstMeta.SrcID),
				// 	eofPayload,
				// 	true,
				// )
				// if err != nil {
				// 	log.Printf("failed to create EOF RTP packet: %v", err)
				// } else if _, err := conn.WriteToUDP(eofPkt, udpAddr); err != nil {
				// 	log.Printf("failed to send EOF packet to %s: %v", addr, err)
				// } else {
				// 	log.Printf("sent EOF RTP packet to %s (seq=%d)", addr, seq)
				// }

				// 次のフレーム受信に向けてリセット
				aggPayloadNon0 = nil
				haveFirstMeta = false
				log.Printf("FirstMeta Reset\n")
				// 必要なら clientFrames をクリアする
				// cfMu.Lock()
				// clientFrames = nil
				// cfMu.Unlock()
			}
		} else {
			// PayloadType != 0 の処理
			aggPayloadNon0 = append(aggPayloadNon0, parsed.Payload...)
			lastParsedNon0 = parsed

			if parsed.Marker {
				udpAddr, _ := net.ResolveUDPAddr("udp", addr)
				// 1フレーム完成
				cfMu.Lock()
				clientFrames = append(clientFrames, aggPayloadNon0)
				lastClientFrameTime = time.Now()
				// 最大保持数を超えたら古いものを落とす
				if len(clientFrames) > maxClientFrames {
					clientFrames = clientFrames[1:]
				}
				framesCount := len(clientFrames)
				cfMu.Unlock()

				// フレームが3つ揃うまでコールバックしない
				if framesCount < minClientFrames {
					// 次のフレーム受信に向けてリセットだけ
					aggPayloadNon0 = nil
					continue
				}

				firstMeta.ExtTimestamp = parsed.ExtTimestamp

				// ここからコールバック実行
				// clientFrames をスレッドセーフにコピー
				cfMu.Lock()
				framesCopy := make([][]byte, framesCount)
				copy(framesCopy, clientFrames)
				cfMu.Unlock()

				// コールバック呼び出し
				respData := cb(firstMeta, Registry, framesCopy)

				var results [][]byte
				seq := uint16(0)
				ts := uint32(0)
				_, configPkt, _, err := CreateRTPPacket(
					lastParsedNon0.PayloadType,
					seq,
					ts,
					lastParsedNon0.SSRC,
					lastParsedNon0.CSRC,
					uint32(firstMeta.DstID),
					uint32(firstMeta.SrcID),
					[]byte("Config"),
					true, // EOFパケットにもマーカーを立てる
				)
				if err != nil {
					log.Printf("failed to create Config RTP packet: %v", err)
				}

				results = append(results, configPkt)
				for idx, frameBytes := range respData {
					// --- (1) フレームは既に JPEG バイト列と仮定 ---
					seq++
					ts++
					encodedData := frameBytes

					// --- (2) MTU 単位で分割 ---
					mtu := 1450
					numChunks := len(encodedData) / mtu
					if len(encodedData)%mtu != 0 {
						numChunks++
					}

					// --- (3) 各チャンクを RTP パケット化 ---
					for i := 0; i < numChunks; i++ {
						start := i * mtu
						end := start + mtu
						if end > len(encodedData) {
							end = len(encodedData)
						}
						chunk := encodedData[start:end]

						// 最後のチャンクのみ Marker=true
						marker := (i == numChunks-1)

						// シーケンス番号／タイムスタンプはフレーム＋チャンク単位で調整
						// seq := req.SequenceNumber + uint16(idx*100+i)
						// ts := req.Timestamp + uint32(idx+1)

						_, rtpPkt, _, err := CreateRTPPacket2(
							uint8(firstMeta.PayloadType),
							seq,
							ts,
							lastParsedNon0.SSRC,
							lastParsedNon0.CSRC,
							chunk,
							marker,
						)
						if err != nil {
							log.Printf("Frame #%d chunk #%d: RTP packetization failed: %v", idx, i, err)
							continue
						}
						results = append(results, rtpPkt)
					}
				}

				// EOF パケット生成＆送信
				eofPayload := []byte("EOF")
				// seq := lastParsedNon0.SequenceNumber + 1
				// ts := lastParsedNon0.Timestamp + 1
				seq++
				ts++
				_, eofPkt, _, err := CreateRTPPacket2(
					lastParsedNon0.PayloadType,
					seq,
					ts,
					lastParsedNon0.SSRC,
					lastParsedNon0.CSRC,
					eofPayload,
					true,
				)
				if err != nil {
					log.Printf("failed to create EOF RTP packet: %v", err)
				}
				results = append(results, eofPkt)

				log.Printf("---\n")
				log.Printf("CallBack Function Fin\n")
				log.Printf("CallBack Function Response Length: %d\n", len(respData))
				log.Printf("---\n")

				// mu2.Lock()
				// Registry = nil
				// mu2.Unlock()

				// 応答送信
				for i, pkt := range results {
					if _, err := conn.WriteToUDP(pkt, udpAddr); err != nil {
						log.Printf("failed to send response packet #%d to %s: %v", i, addr, err)
					}
				}

				// // EOF パケット生成＆送信
				// eofPayload := []byte("EOF")
				// seq := lastParsedNon0.SequenceNumber + 1
				// ts := lastParsedNon0.Timestamp + 1
				// _, eofPkt, _, err := CreateRTPPacket(
				// 	lastParsedNon0.PayloadType,
				// 	seq,
				// 	ts,
				// 	lastParsedNon0.SSRC,
				// 	lastParsedNon0.CSRC,
				// 	uint32(firstMeta.DstID),
				// 	uint32(firstMeta.SrcID),
				// 	eofPayload,
				// 	true,
				// )
				// if err != nil {
				// 	log.Printf("failed to create EOF RTP packet: %v", err)
				// } else if _, err := conn.WriteToUDP(eofPkt, udpAddr); err != nil {
				// 	log.Printf("failed to send EOF packet to %s: %v", addr, err)
				// } else {
				// 	log.Printf("sent EOF RTP packet to %s (seq=%d)", addr, seq)
				// }

				// 次のフレーム受信に向けてリセット
				aggPayloadNon0 = nil
				haveFirstMeta = false
				// 必要なら clientFrames をクリアする
				// cfMu.Lock()
				// clientFrames = nil
				// cfMu.Unlock()
			}
		}
	}
}

// UDPサーバ起動
func UDPServer(cb CallbackFunc, rawCh chan []byte) {

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <arg1> [<arg2>][<arg3>]\n", os.Args[0])
		os.Exit(1)
	}

	// parseEntries で os.Args[1]〜[2] から group1, group2 を作成
	group1 := parseEntries(os.Args[1])

	// var group2 []NodeInfo
	// if len(os.Args) > 2 {
	// 	group2 = parseEntries(os.Args[2])
	// }

	// var group3 []NodeInfo
	// if len(os.Args) > 2 {
	// 	group2 = parseEntries(os.Args[2])
	// }
	address := net.JoinHostPort(group1[0].IP, group1[0].Port)
	serverAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		log.Fatalf("ResolveUDPAddr error: %v", err)
	}

	conn, err := net.ListenUDP("udp", serverAddr)
	if err != nil {
		log.Fatalf("ListenUDP error: %v", err)
	}
	defer conn.Close()

	log.Printf("UDP server listening on %s", address)

	go func() {
		for raw := range rawCh {
			mu2.Lock()
			// 生データをそのまま追加
			Registry = append(Registry, raw)
			// スライドウィンドウ形式で最新 N 件だけ保持
			if len(Registry) > maxFrames {
				Registry = Registry[len(Registry)-maxFrames:]
			}
			mu2.Unlock()
		}
	}()

	buffer := make([]byte, 1500)

	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Printf("Failed to read from UDP: %v", err)
			continue
		}
		clientAddr := addr.String()

		// ここで受信したデータをコピーする
		packet := make([]byte, n)
		copy(packet, buffer[:n])

		mu.Lock()
		ch, exists := clientChannels[clientAddr]
		if !exists {
			ch = make(chan []byte, 1000)
			clientChannels[clientAddr] = ch
			go HandleConnection(clientAddr, conn, ch, cb)
		}
		mu.Unlock()

		ch <- packet
	}
}

// ★ 追加: 最後に受信したフレームやリストを取得するための関数
func GetLatestData() []byte {
	mu2.Lock()
	defer mu2.Unlock()

	// Registry が空なら nil を返す
	if len(Registry) == 0 {
		return nil
	}
	// 最新(末尾)のフレームを取得
	lastFrame := Registry[len(Registry)-1]

	// コピーを作成し安全に返す
	copyFrame := append([]byte(nil), lastFrame...)

	return copyFrame
}

func parseEntries(arg string) []NodeInfo {
	var infos []NodeInfo
	// "/" で複数設定を分割
	entries := strings.Split(arg, "/")
	for _, entry := range entries {
		parts := strings.Split(entry, ",")
		if len(parts) < 3 {
			fmt.Fprintf(os.Stderr, "warning: invalid entry %q, skipping\n", entry)
			continue
		}
		numericID := strings.TrimPrefix(parts[2], "node_")
		id, _ := strconv.Atoi(numericID)

		infos = append(infos, NodeInfo{
			IP:     parts[0],
			Port:   parts[1],
			NodeID: id,
		})
	}
	return infos
}
